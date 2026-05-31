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
	schemaVersion = 1
	dbDriver      = "sqlite"
)

// initDBTables creates the required tables if they do not exist.
func initDBTables(db *sql.DB) error {
	createMetadata := `
	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`

	createFiles := `
	CREATE TABLE IF NOT EXISTS files (
		path TEXT PRIMARY KEY,
		file_hash TEXT NOT NULL,
		public_key TEXT NOT NULL,
		signature TEXT NOT NULL,
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	);`

	createVersion := `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);`

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range []string{createMetadata, createFiles, createVersion} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	// Insert schema version if not present
	var ver int
	err = tx.QueryRow("SELECT version FROM schema_version").Scan(&ver)
	if err == sql.ErrNoRows {
		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return tx.Commit()
}

// openDB opens (or creates) the SQLite database, sets pragmas, and ensures tables exist.
func openDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	db, err := sql.Open(dbDriver, dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	if err := initDBTables(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// getMetadata retrieves a string value from the metadata table.
func getMetadata(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// setMetadata inserts or replaces a metadata value.
func setMetadata(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)", key, value)
	return err
}

// upsertFileEntry inserts or replaces a file record.
func upsertFileEntry(db *sql.DB, path, hash, pubKey, sig string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO files (path, file_hash, public_key, signature, updated_at)
		VALUES (?, ?, ?, ?, strftime('%s', 'now'))
	`, path, hash, pubKey, sig)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// getBranchAuthor returns the branch_author from metadata, or empty string.
func getBranchAuthor(db *sql.DB) (string, error) {
	return getMetadata(db, "branch_author")
}

// setBranchAuthor stores the branch_author.
func setBranchAuthor(db *sql.DB, author string) error {
	return setMetadata(db, "branch_author", author)
}

// getBranch returns the current branch.
func getBranch(db *sql.DB) (string, error) {
	return getMetadata(db, "branch")
}

// getSubBranch returns the current sub‑branch.
func getSubBranch(db *sql.DB) (string, error) {
	return getMetadata(db, "sub_branch")
}

// setBranchAndSubBranch updates branch and sub‑branch in a single transaction.
func setBranchAndSubBranch(db *sql.DB, branch, subBranch string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)", "branch", branch)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)", "sub_branch", subBranch)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Key management helpers
func getKeyPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic("cannot get home dir")
	}
	return filepath.Join(homeDir, ".red-signer", "maintainer.key")
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

	const version = "2.1.0" // updated version

	if *versionFlag {
		fmt.Println(version)
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
	// Compute relative path from vault root (parent of .red-signer folder)
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
