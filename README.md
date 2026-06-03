# RED-Feather (Project R.E.D. Network)

**The official authoring bridge for the Project R.E.D. decentralized knowledge base.**

A hybrid TypeScript (Obsidian) + Go plugin that cryptographically signs your Markdown notes with Ed25519 directly inside Obsidian. The plugin manages the UI and vault metadata; a bundled Go binary (`red-feather`) handles all cryptographic operations. It automatically generates and updates the network's `manifest.json`, securely storing the file hash, your public key, your cryptographic signature, and **branch classification** metadata.

---

## 🦅 The Philosophy: Why We Built This

Project R.E.D. is built on a strict engineering philosophy: **stateless, lightweight, and execution‑focused.** When we designed the security grid for the network, we needed a visual interface for maintainers to hash, sign, and manage their guides. The standard industry reaction is to reinvent the wheel – to waste weeks building a bloated, custom desktop application in C++ and Qt6 just to render a file tree and click a "Sign" button.

We rejected that.

Instead of building an authoring environment from scratch, we simply made use of one of the best local‑first Markdown editors in the world: Obsidian. By turning our signing tool into an Obsidian plugin, we achieved zero context‑switching. Maintainers can write their guides, hit a hotkey, and have the Ed25519 cryptography handled completely invisibly in the background.

We build tools that work. We don't reinvent the wheel.

---

## 🏗️ Architecture: TypeScript UI + Go Crypto Backend

**RED-Feather is a hybrid plugin:**

- **TypeScript (Obsidian Plugin)** — provides the user interface, modal dialogs, status bar, and vault metadata management. Runs inside Obsidian.
- **Go Binary (red-feather)** — bundled cryptographic backend that performs all Ed25519 signing, key generation, and manifest operations. Launched by the TypeScript layer via `child_process.execFile()`.

**How they work together:**

1. You open a Markdown file and click "Sign this note" in Obsidian.
2. The TypeScript plugin displays a modal asking you to select a **Knowledge Branch** and **Sub-Branch**.
3. On "Sign", the plugin calls the bundled `red-feather` Go binary with the file path, vault location, and branch metadata.
4. The Go binary:
   - Loads or generates your Ed25519 private key (`~/.red-feather/maintainer.key`)
   - Computes SHA-256 hash of your file
   - Signs the hash with your private key
   - Writes the result to `~/.red-feather/signer.db` (SQLite)
   - Returns your public key (hex-encoded)
5. The TypeScript plugin receives the result, updates the vault's `manifest.json`, and refreshes the status bar.

**Cross-platform support:** The plugin ships with platform-specific binaries (`red-feather-linux-x64`, `red-feather-macos-x64`, `red-feather-windows-x64.exe`, etc.). At startup, it detects your OS and loads the correct binary.

---

## ⚡ Features

- **Zero‑Friction Signing:** Sign your files with one click via the ribbon icon, editor menu, or command palette.
- **The Sovereign Identity Vault:** Automatically generates and stores your permanent Ed25519 private key safely outside your vault (`~/.red-feather/maintainer.key`).
- **Network Manifest Injection:** Automatically discovers and updates the `manifest.json` at your vault root, formatting the keys exactly as the RED-Engine Go server requires.
- **Knowledge Branch Classification:** When signing your first note, you choose a **branch** (e.g., "Formal Sciences") and an optional **sub‑branch** (e.g., "Mathematics/Logic"). These are stored in the manifest and become immutable for that vault – only the original author can change them.
- **Real‑Time Security Status:** A status bar indicator that displays `✓ Signed` in green, `Unsigned` in gray, or `⚠ modified` in yellow by dynamically comparing your live file hash against the manifest.
- **Public Key Clipboard:** Instantly copy your public key to add to the server's `contributors.json` file.
- **Identity Warning:** Automatically creates a `README.md` inside `~/.red-feather/` warning you never to delete your private key.

---

## 🤝 Powered by RED Collective

This plugin is an integral part of the **Project R.E.D. Network**, a collaborative effort by the RED Collective. Our mission is to build a decentralized, sovereign knowledge base, free from centralized control and commercial manipulation. By using this tool, you contribute to a network that values cryptographic integrity, transparent attribution, and community-driven curation.

Learn more about Project R.E.D. and the RED Collective at https://github.com/RED-Collective.

---

## 🛠️ Installation & Setup

Choose the method that fits your workflow.

### 1. Script (recommended)

**Prerequisites:** `npm` must be installed on your system.

#### macOS / Linux

Open a terminal and run:

```bash
bash <(curl -s https://raw.githubusercontent.com/RED-Collective/RED-Feather/main/install-red-feather.sh)
```

#### Windows

Open PowerShell **as Administrator** and run:

```powershell
iex (iwr -UseBasicParsing https://raw.githubusercontent.com/RED-Collective/RED-Feather/main/install-red-feather.ps1).Content
```

The script will:
- Download the latest plugin and the correct binary for your OS.
- Place everything in the right Obsidian plugins folder.
- Make the binary executable (macOS/Linux).

### 2. Manual Installation

1. Go to the [Releases page](https://github.com/RED-Collective/RED-Feather/releases).
2. Download the latest `RED-Feather.zip` **and** the binary for your operating system:

   | Your OS              | Binary Name                  |
   | -------------------- | ---------------------------- |
   | Linux                | `red-feather-linux-x64`      |
   | macOS (Intel)        | `red-feather-macos-x64`      |
   | macOS (Apple Silicon)| `red-feather-macos-arm64`    |
   | Windows              | `red-feather-windows-x64.exe`|

3. Extract `RED-Feather.zip` – you will get a folder named `RED-Feather`.
4. Move the downloaded binary (e.g., `red-feather-linux-x64`) **inside** that `RED-Feather` folder.
5. Move the whole `RED-Feather` folder into your Obsidian vault's plugins directory:  
   `YourVault/.obsidian/plugins/`
6. **macOS / Linux only** – make the binary executable:

   ```bash
   chmod +x /path/to/YourVault/.obsidian/plugins/RED-Feather/red-feather-*
   ```

7. Restart Obsidian (or reload community plugins) and enable **RED-Feather** in `Settings → Community plugins`.

✅ **No renaming needed** – the plugin now uses the exact binary names listed above.

---

## 🚀 The Genesis Workflow (First Use)

1. Open any Markdown note in your vault.
2. Click the **signature icon** (✍️) in the left ribbon, or use the command palette (`Ctrl/Cmd + P` → "Sign current note").
3. A modal will appear showing your **Public Key**. Copy it – you will need to add it to your RED-Engine node's `contributors.json`.
4. Choose a **Knowledge Branch** from the dropdown (e.g., `Formal Sciences`).
5. Choose a **Sub‑Branch** (e.g., `Mathematics/Logic`) – the list adapts to your branch selection.
6. Click **Sign this note**.

**What happens under the hood:**
- If you are a new maintainer, the plugin generates your private key at `~/.red-feather/maintainer.key` (strict `0600` permissions).
- It creates or locates `manifest.json` at the root of your vault.
- The manifest stores:
  ```json
  {
    "branch": "Formal Sciences",
    "sub_branch": "Mathematics/Logic",
    "branch_author": "your_public_key_hex",
    "files": { ... }
  }
  ```
- The `branch_author` field is set **once** – from that moment, **only you** can change the branch or sub‑branch for this vault. Any other user who tries will see a lock icon and receive a warning.
- The status bar will glow `✓ Signed` (green). If you modify even a single character, the hash changes and the status immediately reverts to `Unsigned` until you sign again.

---

## 🛡️ Security Architecture

- **Private Key Storage:** Your Ed25519 private key (`maintainer.key`) is generated and stored at `~/.red-feather/maintainer.key` on your local machine with strict `0600` permissions (owner read/write only). **Never upload this key or share it.** This directory is created outside your vault for security.
- **Key Management:** The bundled Go binary (`red-feather`) loads your key, signs files, and returns signatures to the TypeScript layer. The key never leaves your machine.
- **Vault-Specific Metadata:** Signing history and branch classification are stored per-vault in `.red-feather/signer.db` (SQLite, inside your vault root). This database can be safely backed up or version-controlled.
- **Identity Warning:** A `README.md` is automatically created in `~/.red-feather/`, warning you about the critical importance of your private key.
- **Verification:** The Obsidian plugin only signs the files. Verification happens strictly on the RED-Engine server side, ensuring no compromised files are ever rendered to the end‑user.
- **Branch Author Lock:** Once a `branch_author` is set, the plugin disables the branch/sub‑branch dropdown for anyone whose public key does not match. This prevents tampering with classification after the vault is published.

---

## 💻 Building from Source (For Contributors)

If you want to audit or modify the plugin's TypeScript architecture:

```bash
git clone https://github.com/RED-Collective/RED-Feather
cd RED-Feather
npm install
npm run build   # or `npx tsc`
```

The compiled `main.js` will replace the existing one.

---

## ⚖️ License & Attribution

**RED-Feather** is part of **Project R.E.D.** and is licensed under the
**GNU Affero General Public License v3.0 (AGPL-3.0)** — see [`LICENSE.md`](./LICENSE.md).

Per **AGPL-3.0 Section 7(b)**, an additional attribution term applies: any copy,
modified version, or derivative must preserve the credit

> **Powered by [RED Collective](https://github.com/RED-Collective).**

in the notices the software displays to its users (the plugin's signing modal,
the bundled `red-feather --version` output, and this README). The exact, binding
terms are in [`NOTICE.md`](./NOTICE.md). This credit **may not be removed**.

© 2026 RED Collective · <https://github.com/RED-Collective>

---

## Changelog

### 2026-06-03

**Rebrand → RED-Feather**
- Renamed the plugin and its bundled Go backend from **Red Signer** to **RED-Feather**. The produced executables are now `red-feather-linux-x64`, `red-feather-linux-arm64`, `red-feather-macos-x64`, `red-feather-macos-arm64`, and `red-feather-windows-x64.exe` (previously `signer-*`). The plugin's startup binary detection was updated to match.
- Moved the on-disk identity directory from `~/.red-signer/` to `~/.red-feather/` (private key, public key, identity README) and the per-vault database to `<vault>/.red-feather/signer.db`. Existing users must move their old `~/.red-signer/` folder to `~/.red-feather/` to keep their maintainer identity.
- Updated the `--version` banner to `red-feather`, renamed the Go source directory to `cmd/red-feather`, and renamed the install scripts to `install-red-feather.{sh,ps1}`. The AGPL-3.0 §7(b) "Powered by RED Collective" attribution is preserved unchanged.

### 2026-06-02

**Architecture**
- Formally separated the plugin into two distinct layers: the TypeScript Obsidian plugin handles all UI (modals, status bar, ribbon icon, context menu, command palette) and the bundled `red-signer` Go binary handles all cryptographic operations (key generation, SHA-256 hashing, Ed25519 signing, SQLite writes). The TypeScript layer communicates with the binary exclusively via `child_process.execFile()`.
- Documented cross-platform binary detection at plugin startup: `signer-linux-x64`, `signer-linux-arm64`, `signer-macos-x64`, `signer-macos-arm64`, `signer-windows-x64.exe`. The correct binary is selected by inspecting `process.platform` and `process.arch` at load time.

**Key Directory & Identity**
- Fixed path inconsistency: the private key, public key, and identity README are now consistently stored at `~/.red-signer/` across both the TypeScript plugin and the Go binary. Previously the TypeScript `ensureReadme()` function created the directory at `~/.red-network/`, causing the README to appear in a different location than the key files.

**Database Schema (red-signer)**
- Migration 1: initial `files` table (path, hash, public_key, signature) and `metadata` key-value store.
- Migration 2: indexes on `files.path` and `files.public_key` for faster status lookups.
- Migration 3: introduced `vault_metadata` singleton table, migrating KV pairs from the old `metadata` table (branch, sub_branch, branch_author) into typed columns for direct SQL access.
- Migration 4: added `files.sign_count` integer column, incremented on every successful signing operation for audit trail purposes.

**Branch Author Lock**
- The first signer of a vault becomes the immutable `branch_author`. On subsequent opens, the plugin calls `--get-branch` and compares the returned `branch_author` public key against the current user's key. If they do not match, the branch and sub-branch dropdowns in the signing modal are disabled and a lock indicator is shown.

**Attribution & License**
- AGPL-3.0 §7(b) attribution ("Powered by RED Collective.") enforced in the signing modal footer (`src/main.ts`) and in the `red-signer --version` output. Tagged `Attribution required by NOTICE (AGPL-3.0 §7(b)) — do not remove` at every insertion point.
