# Red Signer Plugin – Complete Architecture Map

This document provides a detailed architectural map of the **Red Signer** Obsidian plugin, covering all components, their interactions, data flows, and external dependencies.

---

## 1. High‑Level Component Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Obsidian Application                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    RedSignerPlugin (TypeScript)                      │   │
│  │  - UI: Ribbon icon, command palette, editor context menu, modal     │   │
│  │  - Status bar indicator (signed / unsigned)                         │   │
│  │  - Event listeners (file modification, active leaf change)          │   │
│  │  - Binary management (path, permissions, execution)                 │   │
│  │  - Database path resolution (.red-signer.db in vault root)          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    │ child_process.execFile                │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                 Go Binary (signer-<platform>-<arch>)                 │   │
│  │  - Ed25519 key generation / loading (keys in ~/.red-network/)       │   │
│  │  - SQLite database operations (create tables, CRUD)                 │   │
│  │  - File hashing (SHA‑256) and signing                               │   │
│  │  - Branch / sub‑branch metadata management                          │   │
│  │  - Author authority enforcement                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │              SQLite Database (.red-signer.db in vault root)          │   │
│  │  Tables: metadata, files, schema_version                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │             File System (outside vault)                              │   │
│  │  - ~/.red-network/maintainer.key (private Ed25519 key)              │   │
│  │  - ~/.red-network/README.md (warning notice)                        │   │
│  │  - <pluginDir>/.pubkey_shown (flag to avoid repeated public key     │   │
│  │    notification)                                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Plugin Lifecycle & Initialisation

### 2.1 `onload()` Sequence

1. **Determine vault root**  
   `this.app.vault.adapter.getBasePath()` – only works on desktop Obsidian (fails otherwise).

2. **Set database path**  
   `path.join(vaultRoot, ".red-signer.db")`

3. **Locate plugin directory**  
   - Try `app.plugins.getPlugin("red-signer")?.manifest?.dir`
   - Fallback: `<vaultRoot>/.obsidian/plugins/red-signer`

4. **Select binary name** based on `process.platform` / `process.arch`:
   - Windows: `signer-windows-x64.exe`
   - macOS (arm64): `signer-macos-arm64`
   - macOS (x64): `signer-macos-x64`
   - Linux: `signer-linux-x64`
   - Fallback: `signer`

5. **Ensure binary executable** (non‑Windows only)  
   - Check `fs.statSync` for mode bits, `fs.chmodSync(0o755)` if needed.

6. **Create warning README** in `~/.red-network` (`ensureReadme()`)

7. **Load current branch info** from database (`loadBranchFromDb()`)  
   - Calls binary with `--db <dbPath> --get-branch`  
   - Parses JSON output into `this.currentBranch`, `this.currentSubBranch`

8. **Add status bar item** and set initial status (`updateStatusForActiveFile()`)

9. **Register event handlers**  
   - `vault.on("modify")` – re‑check status when active file saved  
   - `workspace.on("active-leaf-change")` – update status on tab switch

10. **Add ribbon icon** (ID `signature`) with tooltip *Red Signer: Sign current note*  
    Opens `SignModal` for current markdown file.

11. **Add editor context menu item**  
    “Sign this note directly” – calls `signFile(file)` immediately.

12. **Register commands**  
    - `sign-current-note` – signs current file  
    - `copy-public-key` – copies public key to clipboard

---

## 3. UI Components

### 3.1 `SignModal` (branch selection & signing modal)

**Opened from** ribbon icon (or command if extended).

**Layout & behaviour**:
- Shows current file name.
- Displays **public key** (from `getPublicKey()`) with copy button.
- If user is **branch author** (`isBranchAuthor()`):
  - Dropdown for **Knowledge Branch** (predefined list: Formal Sciences, Physical Sciences, …)
  - Dropdown for **Sub‑Branch** (dynamic based on main branch selection)
  - Changing branch/sub‑branch triggers `updateBranch()` after confirmation.
- If user is **not** author and a branch is already set:
  - Dropdowns disabled, lock message shown.
- **Sign button**:
  - If author, first updates branch (if changed).
  - Then calls `signFile()`.
- **Close button**.

### 3.2 Status Bar

- Displays `✓ Signed` (green) or `Unsigned` (muted).
- Updated via `updateStatusForActiveFile()`:
  - Debounced (100 ms) to avoid excessive binary calls.
  - Calls binary with `--check-status <filePath>` – **Note:** the provided `main.go` does **not** implement `--check-status`. The plugin therefore always falls into the error branch (stderr contains “no such table” or unknown flag), showing “Unsigned”. This is a mismatch; the binary would need to be extended or the plugin adjusted.

---

## 4. Key Management (Go binary)

### 4.1 Storage Location

- `~/.red-network/maintainer.key` – **private** Ed25519 key (permissions `0600`).
- `~/.red-network/maintainer.pub` – public key in hex (optional, created for convenience).

### 4.2 Key Generation / Loading (`ensureKeys()`)

- If `maintainer.key` exists → load it.
- Otherwise generate a new Ed25519 key pair using `crypto/rand`, write private key, and output the public key to stderr (and to `.pub` file).

### 4.3 Public Key Retrieval (`--print-pubkey`)

- Reads private key, derives public key, prints hex‑encoded string to stdout.

---

## 5. SQLite Database Schema

**File**: `.red-signer.db` (in vault root)

### 5.1 Table `metadata`
| Column | Type    | Description                          |
|--------|---------|--------------------------------------|
| `key`  | TEXT PK | e.g. `branch`, `sub_branch`, `branch_author` |
| `value`| TEXT    | Corresponding value                   |

### 5.2 Table `files`
| Column       | Type    | Description                                 |
|--------------|---------|---------------------------------------------|
| `path`       | TEXT PK | Relative path from DB directory (vault root)|
| `file_hash`  | TEXT    | SHA‑256 hash of file (hex)                  |
| `public_key` | TEXT    | Public key of signer (hex)                  |
| `signature`  | TEXT    | Ed25519 signature of hash (hex)             |
| `updated_at` | INTEGER | Unix timestamp (default `strftime('%s','now')`) |

### 5.3 Table `schema_version`
| Column    | Type    | Description          |
|-----------|---------|----------------------|
| `version` | INTEGER PK | Current schema version (1) |

---

## 6. Branch Classification & Author Authority

### 6.1 Concepts

- **Branch author** – The public key that first initialises the database or first sets a branch.  
  Stored in `metadata` key `branch_author`.
- **Only the branch author** may change the branch/sub‑branch classification for the whole vault.
- If no branch has been set, the **first signer** automatically becomes the branch author and the branch can be chosen during signing.

### 6.2 Database Operations (binary flags)

| Flag                      | Behaviour                                                                 |
|---------------------------|---------------------------------------------------------------------------|
| `--get-branch`            | Returns JSON: `{"branch": "…", "sub_branch": "…", "branch_author": "…"}`  |
| `--set-branch --branch X --sub-branch Y` | Updates branch & sub‑branch; rejects if caller is not branch author.      |
| `--init --branch X --sub-branch Y`       | First‑time initialisation (also sets author).                             |
| Default (signing)         | If no `branch_author` exists, sets it to the current signer.              |

### 6.3 Plugin Integration

- `loadBranchFromDb()` → populates `currentBranch`, `currentSubBranch`.
- `isBranchAuthor()` → compares `branch_author` from DB with own public key.
- `updateBranch()` → calls binary with `--set-branch` after user confirmation.

---

## 7. Signing a File (Core Workflow)

### 7.1 Plugin Side (`signFile(file)`)

1. Validate binary exists.
2. Get absolute file path via `adapter.getFullPath(file.path)`.
3. Show notice “Signing …”.
4. Spawn binary:  
   `execFile(binaryPath, ["--db", dbPath, fullPath])`
5. On success:  
   - Notice “Signed: …”
   - Reload branch info (`loadBranchFromDb()`)
   - Update status bar
   - Optionally show public key if first signature (`showPublicKeyIfNew()`)
6. On error: show failure notice.

### 7.2 Binary Side (default mode, no special flags)

1. Ensure keys (`ensureKeys()`).
2. Open database (`openDB()`).
3. Read the markdown file, compute SHA‑256 hash.
4. Sign hash with private key (Ed25519).
5. Compute **relative path** from database directory to the file (must be inside vault).
6. Upsert record into `files` table.
7. If no branch author exists, set it to the current signer’s public key.
8. Print success message to stderr.

### 7.3 Security / Integrity

- Only the file **hash** is signed, not the full content.
- The database stores the hash, public key, and signature for every signed file.
- Verification (not yet implemented in plugin) would re‑hash the file and check the signature against the stored public key.

---

## 8. Binary Command Summary (Flags)

| Flag                    | Required               | Description                                                                 |
|-------------------------|------------------------|-----------------------------------------------------------------------------|
| `--db PATH`             | For all except `--print-pubkey` and `--version` | Path to SQLite database.          |
| `--print-pubkey`        | –                      | Print public key (hex) and exit.                                            |
| `--version`             | –                      | Print binary version (2.0.0) and exit.                                      |
| `--get-branch`          | `--db`                 | Output JSON with branch, sub‑branch, branch_author.                         |
| `--set-branch`          | `--db`, `--branch`     | Update branch and optional sub‑branch. Author check enforced.               |
| `--init`                | `--db`, `--branch`     | Initialise DB with branch metadata and set author.                          |
| `--branch VALUE`        | Varies                 | Main branch name.                                                           |
| `--sub-branch VALUE`    | Optional               | Sub‑branch name.                                                            |
| *no flag*               | `--db` + one file arg  | Sign the given file (default mode).                                         |

---

## 9. Discrepancies & Missing Features

### 9.1 `--check-status` Not Implemented in Go Binary

- The plugin calls `--check-status <filePath>` to determine if a file is signed.
- The provided `main.go` has **no** such flag → the binary will exit with an error, causing the plugin to always display “Unsigned”.
- **Fix needed:** Implement a `--check-status` flag that queries the `files` table and returns “signed” if the file’s hash matches the stored hash (or just if an entry exists).

### 9.2 No Verification Command

- The plugin currently cannot verify signatures (only sign). A `--verify` flag would be needed for a full trust workflow.

### 9.3 Branch Options Hardcoded

- `subBranchOptions` in TypeScript is hardcoded. The binary has no knowledge of allowed branches; enforcement is only via the UI and the binary’s author check.

### 9.4 Database Path Security

- The binary requires that the signed file reside **inside** the database directory (vault root). This is enforced via `filepath.Rel` and a check for `".."`. This prevents signing files outside the vault.

### 9.5 Concurrent Access

- SQLite is opened with `busy_timeout=5000` and `WAL` mode, which is safe for multiple processes. However, the plugin may spawn several binary instances concurrently (e.g. status checks while signing). The binary does **not** implement any locking beyond SQLite’s own mechanisms – this should be fine.

---

## 10. Data Flow Diagram (Signing a Note)

```
[User clicks "Sign this note"]
         │
         ▼
┌─────────────────────┐
│  signFile(file)     │
│  (TypeScript)       │
└─────────┬───────────┘
          │ child_process.execFile
          │   args: [ "--db", ".red-signer.db", fullPath ]
          ▼
┌─────────────────────────────────────────┐
│  Go binary (default mode)               │
│  1. Load / generate key pair            │
│  2. Open SQLite DB                      │
│  3. Read file → SHA‑256 hash            │
│  4. Sign hash → signature               │
│  5. Compute relative file path          │
│  6. UPSERT into files table             │
│  7. If no branch_author, set to signer  │
│  8. Print success                       │
└─────────┬───────────────────────────────┘
          │ stdout / stderr
          ▼
┌─────────────────────┐
│  Plugin handles     │
│  result, updates UI │
└─────────────────────┘
```

---

## 11. File System Layout (Example)

```
Vault Root/
├── .red-signer.db          (SQLite database)
├── .obsidian/
│   └── plugins/
│       └── red-signer/
│           ├── main.js
│           ├── manifest.json (not shown, but required)
│           ├── signer-windows-x64.exe
│           ├── signer-macos-arm64
│           ├── signer-macos-x64
│           ├── signer-linux-x64
│           └── .pubkey_shown   (flag file)
└── (any markdown files)

User Home (~)/
└── .red-network/
    ├── maintainer.key       (private key, 0600)
    ├── maintainer.pub       (public key hex, 0644)
    └── README.md            (warning notice)
```

---

## 12. Summary of Interactions

| Component A            | Action                                        | Component B            |
|------------------------|-----------------------------------------------|------------------------|
| Obsidian user         | Click ribbon icon / run command               | `RedSignerPlugin`      |
| `RedSignerPlugin`     | Spawn `execFile` with `--print-pubkey`        | Go binary              |
| `RedSignerPlugin`     | Spawn `execFile` with `--get-branch`          | Go binary + SQLite     |
| `RedSignerPlugin`     | Spawn `execFile` with `--set-branch ...`      | Go binary + SQLite     |
| `RedSignerPlugin`     | Spawn `execFile` with file path               | Go binary + SQLite + FS|
| Go binary             | Read / write `~/.red-network/maintainer.key`  | File system            |
| Go binary             | Read/write `.red-signer.db`                   | SQLite engine          |
| Go binary             | Read markdown file                            | File system            |

---

This architecture map covers all interactions, from user interface events down to cryptographic operations and persistent storage. The main outstanding issue is the missing `--check-status` implementation in the Go binary, which prevents the status bar from correctly reflecting whether a file has been signed.