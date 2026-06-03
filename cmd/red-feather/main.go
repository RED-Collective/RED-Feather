package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type ManifestEntry struct {
	FileHash  string `json:"file_hash"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

const (
	dbDriver = "sqlite"
)

// migration is a single, ordered, idempotent schema change. Each one runs inside
// its own transaction and is recorded in schema_migrations on success.
type migration struct {
	version     int
	description string
	apply       func(tx *sql.Tx) error
}

// migrations is the ordered list of schema changes. The current schema version is
// MAX(version) in schema_migrations. Never reorder or renumber existing entries —
// only append new ones.
var migrations = []migration{
	{
		version:     1,
		description: "initial schema: files + metadata KV",
		apply: func(tx *sql.Tx) error {
			stmts := []string{
				`CREATE TABLE IF NOT EXISTS files (
					path TEXT PRIMARY KEY,
					file_hash TEXT NOT NULL,
					public_key TEXT NOT NULL,
					signature TEXT NOT NULL,
					updated_at INTEGER DEFAULT (strftime('%s', 'now'))
				)`,
				`CREATE TABLE IF NOT EXISTS metadata (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL
				)`,
			}
			for _, s := range stmts {
				if _, err := tx.Exec(s); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     2,
		description: "add idx_files_public_key and idx_files_updated_at",
		apply: func(tx *sql.Tx) error {
			stmts := []string{
				`CREATE INDEX IF NOT EXISTS idx_files_public_key ON files(public_key)`,
				`CREATE INDEX IF NOT EXISTS idx_files_updated_at ON files(updated_at DESC)`,
			}
			for _, s := range stmts {
				if _, err := tx.Exec(s); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     3,
		description: "migrate metadata KV to vault_metadata singleton",
		apply: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS vault_metadata (
					id             INTEGER  PRIMARY KEY CHECK(id = 1),
					branch         TEXT     NOT NULL DEFAULT '',
					sub_branch     TEXT     NOT NULL DEFAULT '',
					branch_author  TEXT     NOT NULL DEFAULT '',
					initialized_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
				)`); err != nil {
				return err
			}

			// Carry over any values from the legacy metadata KV bag.
			branch, subBranch, author := "", "", ""
			rows, err := tx.Query(`SELECT key, value FROM metadata`)
			if err == nil {
				for rows.Next() {
					var k, v string
					if err := rows.Scan(&k, &v); err != nil {
						rows.Close()
						return err
					}
					switch k {
					case "branch":
						branch = v
					case "sub_branch":
						subBranch = v
					case "branch_author":
						author = v
					}
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
			}

			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO vault_metadata (id, branch, sub_branch, branch_author)
				VALUES (1, ?, ?, ?)`, branch, subBranch, author); err != nil {
				return err
			}

			// Retire the legacy tables now that everything is in vault_metadata.
			for _, s := range []string{
				`DROP TABLE IF EXISTS metadata`,
				`DROP TABLE IF EXISTS schema_version`,
			} {
				if _, err := tx.Exec(s); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     4,
		description: "add files.sign_count",
		apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE files ADD COLUMN sign_count INTEGER NOT NULL DEFAULT 1`)
			if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
				return err
			}
			return nil
		},
	},
}

// appliedVersions returns the set of migration versions already recorded.
func appliedVersions(db *sql.DB) (map[int]bool, error) {
	applied := make(map[int]bool)
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// tableExists reports whether a table with the given name is present.
func tableExists(db *sql.DB, name string) bool {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
	return err == nil
}

// runMigrations applies any unapplied migrations in order. Each migration runs in
// its own transaction so a failure leaves earlier migrations intact.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER  NOT NULL PRIMARY KEY,
			applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			description TEXT     NOT NULL DEFAULT ''
		)`); err != nil {
		return err
	}

	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	// Adopt a pre-migration-system database: if files already exists but nothing is
	// recorded, treat the legacy v1 schema (files + metadata) as migration 1 so the
	// remaining migrations transform it forward instead of re-creating tables.
	if len(applied) == 0 && tableExists(db, "files") {
		if _, err := db.Exec(
			`INSERT INTO schema_migrations (version, description) VALUES (1, ?)`,
			migrations[0].description); err != nil {
			return err
		}
		applied[1] = true
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := m.apply(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s) failed: %w", m.version, m.description, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, description) VALUES (?, ?)`,
			m.version, m.description); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d failed: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d failed: %w", m.version, err)
		}
	}
	return nil
}

// ensureVaultMetadataRow guarantees the singleton vault_metadata row (id = 1) exists.
func ensureVaultMetadataRow(db *sql.DB) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO vault_metadata (id) VALUES (1)`)
	return err
}

// openDB opens (or creates) the SQLite database, sets pragmas, and runs migrations.
func openDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	db, err := sql.Open(dbDriver, dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureVaultMetadataRow(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// getVaultMetadata reads the singleton vault_metadata row.
func getVaultMetadata(db *sql.DB) (branch, subBranch, author string, err error) {
	err = db.QueryRow(`SELECT branch, sub_branch, branch_author FROM vault_metadata WHERE id = 1`).
		Scan(&branch, &subBranch, &author)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	return branch, subBranch, author, err
}

// upsertFileEntry inserts a file record or updates the existing one, bumping
// sign_count each time the same path is re-signed.
func upsertFileEntry(db *sql.DB, path, hash, pubKey, sig string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO files (path, file_hash, public_key, signature, updated_at, sign_count)
		VALUES (?, ?, ?, ?, strftime('%s', 'now'), 1)
		ON CONFLICT(path) DO UPDATE SET
			file_hash  = excluded.file_hash,
			public_key = excluded.public_key,
			signature  = excluded.signature,
			updated_at = excluded.updated_at,
			sign_count = files.sign_count + 1
	`, path, hash, pubKey, sig)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// getBranchAuthor returns the branch_author from vault_metadata, or empty string.
func getBranchAuthor(db *sql.DB) (string, error) {
	_, _, author, err := getVaultMetadata(db)
	return author, err
}

// setBranchAuthor stores the branch_author on the singleton row.
func setBranchAuthor(db *sql.DB, author string) error {
	_, err := db.Exec(
		`UPDATE vault_metadata SET branch_author = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`,
		author)
	return err
}

// getBranch returns the current branch.
func getBranch(db *sql.DB) (string, error) {
	branch, _, _, err := getVaultMetadata(db)
	return branch, err
}

// getSubBranch returns the current sub‑branch.
func getSubBranch(db *sql.DB) (string, error) {
	_, subBranch, _, err := getVaultMetadata(db)
	return subBranch, err
}

// setBranchAndSubBranch updates branch and sub‑branch on the singleton row.
func setBranchAndSubBranch(db *sql.DB, branch, subBranch string) error {
	_, err := db.Exec(
		`UPDATE vault_metadata SET branch = ?, sub_branch = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`,
		branch, subBranch)
	return err
}

// Key management helpers
func getKeyPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic("cannot get home dir")
	}
	return filepath.Join(homeDir, ".red-feather", "maintainer.key")
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read private key: %v", err)
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size")
	}
	return ed25519.PrivateKey(data), nil
}

func ensureKeys() (ed25519.PrivateKey, ed25519.PublicKey) {
	keyPath := getKeyPath()
	keyDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot create key dir: %v\n", err)
		os.Exit(1)
	}

	privKey, err := loadPrivateKey(keyPath)
	if err == nil {
		pubKey := privKey.Public().(ed25519.PublicKey)
		fmt.Fprintln(os.Stderr, "[SYS] Loaded existing identity.")
		return privKey, pubKey
	}

	fmt.Fprintln(os.Stderr, "[SYS] Generating new identity...")
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Key generation failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(keyPath, privKey, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot save private key: %v\n", err)
		os.Exit(1)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	pubKeyPath := filepath.Join(keyDir, "maintainer.pub")
	_ = os.WriteFile(pubKeyPath, []byte(pubKeyHex), 0644)
	fmt.Fprintf(os.Stderr, "[NEW IDENTITY] Public key:\n%s\n", pubKeyHex)
	return privKey, pubKey
}

func checkFileStatus(db *sql.DB, vaultRoot, filePath string) (string, error) {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	relPath, err := filepath.Rel(vaultRoot, absFile)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("file is outside vault root")
	}
	relPath = filepath.ToSlash(relPath)

	var storedHash string
	err = db.QueryRow("SELECT file_hash FROM files WHERE path = ?", relPath).Scan(&storedHash)
	if err == sql.ErrNoRows {
		return "unsigned", nil
	}
	if err != nil {
		return "", err
	}

	// Compute current file hash
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	currentHash := sha256.Sum256(fileBytes)
	currentHashHex := hex.EncodeToString(currentHash[:])

	if storedHash == currentHashHex {
		return "signed", nil
	}
	return "modified", nil
}

func main() {
	// Flags
	dbFlag := flag.String("db", "", "Path to SQLite database (required for all operations except --print-pubkey and --version)")
	initFlag := flag.Bool("init", false, "Initialize a new database with branch classification (requires --db and --branch)")
	printPubKeyFlag := flag.Bool("print-pubkey", false, "Print the current public key and exit")
	branchFlag := flag.String("branch", "", "Knowledge branch (required with --init or --set-branch)")
	subBranchFlag := flag.String("sub-branch", "", "Optional sub‑branch path")
	setBranchFlag := flag.Bool("set-branch", false, "Update branch/sub‑branch in DB (requires --db, --branch)")
	getBranchFlag := flag.Bool("get-branch", false, "Print branch/sub‑branch/author from DB (requires --db)")
	versionFlag := flag.Bool("version", false, "Print binary version and exit")
	checkStatusFlag := flag.String("check-status", "", "Check status of a file: 'signed', 'unsigned', or 'modified' (requires --db)")

	flag.Parse()

	const version = "2.2.0" // schema_migrations + vault_metadata + files.sign_count

	if *versionFlag {
		// Attribution required by NOTICE (AGPL-3.0 §7(b)) — do not remove.
		fmt.Println("red-feather " + version)
		fmt.Println("Powered by RED Collective — https://github.com/RED-Collective")
		return
	}

	if *printPubKeyFlag {
		privKey, err := loadPrivateKey(getKeyPath())
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
			os.Exit(1)
		}
		pubKey := privKey.Public().(ed25519.PublicKey)
		fmt.Println(hex.EncodeToString(pubKey))
		return
	}

	// All other operations require --db
	if *dbFlag == "" {
		fmt.Fprintf(os.Stderr, "[ERROR] --db is required for this operation\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Ensure keys exist (for signing or author checks)
	privKey, pubKey := ensureKeys()
	currentPubKeyHex := hex.EncodeToString(pubKey)

	// Open database (creates if missing)
	db, err := openDB(*dbFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Handle --check-status
	if *checkStatusFlag != "" {
		vaultRoot := filepath.Dir(filepath.Dir(*dbFlag))
		status, err := checkFileStatus(db, vaultRoot, *checkStatusFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
			os.Exit(1)
		}
		fmt.Println(status)
		return
	}

	// Handle --get-branch
	if *getBranchFlag {
		branch, _ := getBranch(db)
		subBranch, _ := getSubBranch(db)
		author, _ := getBranchAuthor(db)
		output := map[string]string{
			"branch":        branch,
			"sub_branch":    subBranch,
			"branch_author": author,
		}
		data, _ := json.Marshal(output)
		fmt.Println(string(data))
		return
	}

	// Handle --set-branch
	if *setBranchFlag {
		if *branchFlag == "" {
			fmt.Fprintf(os.Stderr, "[ERROR] --branch is required when using --set-branch\n")
			os.Exit(1)
		}
		storedAuthor, err := getBranchAuthor(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Cannot read branch author: %v\n", err)
			os.Exit(1)
		}
		if storedAuthor != "" && storedAuthor != currentPubKeyHex {
			fmt.Fprintf(os.Stderr, "[ERROR] Only the branch author (%s) can change classification.\n", storedAuthor)
			os.Exit(1)
		}
		if err := setBranchAndSubBranch(db, *branchFlag, *subBranchFlag); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to update branch: %v\n", err)
			os.Exit(1)
		}
		if storedAuthor == "" {
			if err := setBranchAuthor(db, currentPubKeyHex); err != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] Failed to set branch author: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("[SUCCESS] Branch updated to %s/%s\n", *branchFlag, *subBranchFlag)
		return
	}

	// Handle --init
	if *initFlag {
		if *branchFlag == "" {
			fmt.Fprintf(os.Stderr, "[ERROR] --branch is required when using --init\n")
			os.Exit(1)
		}
		if err := setBranchAndSubBranch(db, *branchFlag, *subBranchFlag); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to init DB metadata: %v\n", err)
			os.Exit(1)
		}
		if err := setBranchAuthor(db, currentPubKeyHex); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to set branch author: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[SUCCESS] Initialised database at %s with branch=%s, sub_branch=%s\n", *dbFlag, *branchFlag, *subBranchFlag)
		return
	}

	// Otherwise, signing a file
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s --db <database> [flags] <markdown-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}
	markdownPath := args[0]

	// Read file and compute hash
	fileBytes, err := os.ReadFile(markdownPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot read markdown file: %v\n", err)
		os.Exit(1)
	}
	hash := sha256.Sum256(fileBytes)
	hashHex := hex.EncodeToString(hash[:])
	signature := ed25519.Sign(privKey, hash[:])
	sigHex := hex.EncodeToString(signature)

	// Compute relative path from database directory
	// Compute relative path from vault root (parent of database directory)
	// Compute relative path from vault root (parent of .red-feather folder)
	vaultRoot := filepath.Dir(filepath.Dir(*dbFlag)) // ← changed
	absFile, _ := filepath.Abs(markdownPath)
	relPath, err := filepath.Rel(vaultRoot, absFile)
	if err != nil || strings.HasPrefix(relPath, "..") {
		fmt.Fprintf(os.Stderr, "[ERROR] File is outside the vault root: %v\n", err)
		os.Exit(1)
	}
	relPath = filepath.ToSlash(relPath)

	// Upsert into database
	if err := upsertFileEntry(db, relPath, hashHex, currentPubKeyHex, sigHex); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to update database: %v\n", err)
		os.Exit(1)
	}

	// Set branch author if not set (first signature)
	author, _ := getBranchAuthor(db)
	if author == "" {
		if err := setBranchAuthor(db, currentPubKeyHex); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Could not set branch author: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[INFO] You are now the branch author of this vault.\n")
		}
	}

	fmt.Printf("[SUCCESS] Signed %s\n", relPath)
}
