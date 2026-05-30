package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ManifestEntry struct {
	FileHash  string `json:"file_hash"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type Manifest struct {
	Branch       string                   `json:"branch,omitempty"`
	SubBranch    string                   `json:"sub_branch,omitempty"`
	BranchAuthor string                   `json:"branch_author,omitempty"`
	Files        map[string]ManifestEntry `json:"files"`
}

func main() {
	manifestFlag := flag.String("manifest", "", "Explicit path to authors.json (optional)")
	initFlag := flag.Bool("init", false, "Initialize a brand new manifest in the current directory")
	printPubKeyFlag := flag.Bool("print-pubkey", false, "Print the current public key and exit")
	branchFlag := flag.String("branch", "", "Knowledge branch (required with --init)")
	subBranchFlag := flag.String("sub-branch", "", "Optional sub‑branch path (with --init)")
	flag.Parse()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot find home dir: %v\n", err)
		os.Exit(1)
	}
	keyDir := filepath.Join(homeDir, ".red-network")
	keyPath := filepath.Join(keyDir, "maintainer.key")
	pubKeyPath := filepath.Join(keyDir, "maintainer.pub")

	if *printPubKeyFlag {
		privKey, err := loadPrivateKey(keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
			os.Exit(1)
		}
		pubKey := privKey.Public().(ed25519.PublicKey)
		fmt.Println(hex.EncodeToString(pubKey))
		return
	}

	args := flag.Args()
	if len(args) != 1 && !*initFlag {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <markdown-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Key management (unchanged)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot create key dir: %v\n", err)
		os.Exit(1)
	}
	var privKey ed25519.PrivateKey
	var pubKey ed25519.PublicKey
	keyBytes, err := os.ReadFile(keyPath)
	if err == nil && len(keyBytes) == ed25519.PrivateKeySize {
		privKey = ed25519.PrivateKey(keyBytes)
		pubKey = privKey.Public().(ed25519.PublicKey)
		fmt.Fprintln(os.Stderr, "[SYS] Loaded existing identity.")
		pubKeyHex := hex.EncodeToString(pubKey)
		_ = os.WriteFile(pubKeyPath, []byte(pubKeyHex), 0644)
	} else {
		fmt.Fprintln(os.Stderr, "[SYS] Generating new identity...")
		pubKey, privKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Key generation failed: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(keyPath, privKey, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Cannot save private key: %v\n", err)
			os.Exit(1)
		}
		pubKeyHex := hex.EncodeToString(pubKey)
		_ = os.WriteFile(pubKeyPath, []byte(pubKeyHex), 0644)
		fmt.Fprintf(os.Stderr, "[NEW IDENTITY] Public key:\n%s\n", pubKeyHex)
	}

	markdownPath := ""
	if len(args) == 1 {
		markdownPath = args[0]
	}

	manifestPath := *manifestFlag
	if manifestPath == "" && markdownPath != "" {
		manifestPath, err = findManifest(markdownPath)
		if err != nil {
			if *initFlag {
				cwd, _ := os.Getwd()
				manifestPath = filepath.Join(cwd, "authors.json")
				fmt.Fprintf(os.Stderr, "[SYS] Creating new authors file at: %s\n", manifestPath)
			} else {
				fmt.Fprintf(os.Stderr, "[ERROR] File authors.json not found. Use --init to create one.\n")
				os.Exit(1)
			}
		}
	}

	currentPubKeyHex := hex.EncodeToString(pubKey)

	if *initFlag {
		if *branchFlag == "" {
			fmt.Fprintf(os.Stderr, "[ERROR] --branch is required when using --init\n")
			os.Exit(1)
		}
		manifest := Manifest{
			Branch:       *branchFlag,
			SubBranch:    *subBranchFlag,
			BranchAuthor: currentPubKeyHex,
			Files:        make(map[string]ManifestEntry),
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to create manifest: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Cannot write manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[SUCCESS] Initialized manifest with branch=%s, sub_branch=%s\n", *branchFlag, *subBranchFlag)
		return
	}

	if markdownPath == "" {
		fmt.Fprintf(os.Stderr, "[ERROR] Markdown file required (unless --init)\n")
		os.Exit(1)
	}

	fileBytes, err := os.ReadFile(markdownPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot read markdown file: %v\n", err)
		os.Exit(1)
	}
	hash := sha256.Sum256(fileBytes)
	hashHex := hex.EncodeToString(hash[:])
	signature := ed25519.Sign(privKey, hash[:])

	absMarkdownPath, _ := filepath.Abs(markdownPath)
	relPath, err := filepath.Rel(filepath.Dir(manifestPath), absMarkdownPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Cannot compute relative path: %v\n", err)
		os.Exit(1)
	}
	relPath = filepath.Clean(relPath)
	if strings.HasPrefix(relPath, "..") {
		fmt.Fprintf(os.Stderr, "[ERROR] Markdown file is outside the manifest directory boundary: %s\n", relPath)
		os.Exit(1)
	}
	mapKey := filepath.ToSlash(relPath)

	if err := atomicUpdateManifest(manifestPath, mapKey, hashHex, currentPubKeyHex, hex.EncodeToString(signature)); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to update manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[SUCCESS] Signed %s\n", mapKey)
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

func findManifest(startPath string) (string, error) {
	dir := filepath.Dir(startPath)
	for {
		candidate := filepath.Join(dir, "authors.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no authors.json found")
		}
		dir = parent
	}
}

func atomicUpdateManifest(manifestPath, key, hashHex, pubKeyHex, sigHex string) error {
	var manifest Manifest
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("manifest.json corrupted: %v", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// If this is the first file and BranchAuthor not set, set it to the current public key
	if manifest.BranchAuthor == "" && manifest.Branch != "" {
		manifest.BranchAuthor = pubKeyHex
	}

	if manifest.Files == nil {
		manifest.Files = make(map[string]ManifestEntry)
	}
	manifest.Files[key] = ManifestEntry{
		FileHash:  hashHex,
		PublicKey: pubKeyHex,
		Signature: sigHex,
	}

	updatedData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(manifestPath)
	tmpFile, err := os.CreateTemp(dir, "manifest.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(updatedData); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, manifestPath)
}
