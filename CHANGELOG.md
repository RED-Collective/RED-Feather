# Changelog

All notable changes to the **Obsidian RED-Feather** plugin (formerly Obsidian Red Signer) will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.1.0] – 2026-06-03

### Changed
- **Rebrand to RED-Feather** – the plugin and its bundled Go backend were renamed from **Red Signer** to **RED-Feather**.
- **Executable names** – the produced binaries are now `red-feather-linux-x64`, `red-feather-linux-arm64`, `red-feather-macos-x64`, `red-feather-macos-arm64`, and `red-feather-windows-x64.exe` (previously `signer-*`). The plugin's startup binary detection was updated to match.
- **On-disk paths** – the identity directory moved from `~/.red-signer/` to `~/.red-feather/`, and the per-vault database to `<vault>/.red-feather/signer.db`. Existing installs must move their old `~/.red-signer/` folder to keep their maintainer identity.
- **Internals** – the `--version` banner now reads `red-feather`, the Go source directory is `cmd/red-feather`, and the install scripts are `install-red-feather.{sh,ps1}`. The AGPL-3.0 §7(b) "Powered by RED Collective" attribution is preserved unchanged.

## [2.0.0] – 2026-05-31

### Added
- **SQLite database backend** – all signatures and metadata now stored in `.red-signer/signer.db` inside the vault root (hidden folder). No more `manifest.json`.
- **Full database integration** – the Go binary manages tables for files, metadata, and schema version.
- **Branch and sub‑branch enforcement** – users must explicitly select a branch and sub‑branch before signing the first note. No default selection; sign button remains disabled until both are chosen.
- **Branch author locking** – the database stores `branch_author` (public key of the first signer). Only that author can later modify the branch or sub‑branch.
- **Dynamic sub‑branch dropdown** – sub‑branch options change based on the selected branch (predefined lists for Formal Sciences, Physical Sciences, Social Sciences, Applied Sciences, Arts & Humanities, Philosophy & Ethics).
- **Automatic README** – a `README.md` is created in `~/.red-network/` warning about the importance of the private key.
- **Platform‑specific binary detection** – the plugin now expects exact binary names (`signer-linux-x64`, `signer-linux-arm64`, `signer-macos-x64`, `signer-macos-arm64`, `signer-windows-x64.exe`).
- **Lock icon and warning** in the modal when a user is not the branch author.
- **Binary `--check-status` flag** – allows the plugin to query whether a file is signed, unsigned, or modified without reading the file content directly.
- **Status bar** now shows “✓ Signed” (green), “⚠ Modified” (orange), or “Unsigned” (muted) based on the database entry and current file hash.

### Changed
- **Complete migration from JSON manifest to SQLite** – all operations (signing, branch management, status checks) now use the database exclusively.
- **Database location** moved from vault root `.red-signer.db` to vault root `.red-signer/signer.db` – keeps the root cleaner.
- **Vault root detection** improved – now works with multiple Obsidian adapter methods.
- **Signing workflow** – the binary now expects the database to be in a subfolder; relative paths are computed from the vault root (parent of `.red-signer/`).
- **Error handling** – clearer messages when binary is missing or permissions are wrong.
- **UI modal** – sign button initially disabled; enabled only after valid branch and sub‑branch are selected (first‑time users). Existing branch authors can still change classification.
- **Direct signing (command/context menu)** now opens the modal if no branch exists, forcing branch selection before any signature.

### Fixed
- **Path traversal bug** – when database was inside a subfolder (`.red-signer/`), files in the vault root were incorrectly rejected as “outside the vault root”. Fixed by computing relative paths from the vault root (two levels up from the database file).
- **Race condition** – multiple simultaneous operations now safe thanks to SQLite WAL mode and busy timeout.
- **Missing executable permission** on Unix systems – the plugin automatically runs `chmod +x` on the binary.
- **Unused variables** removed from `SignModal` class (`selectedBranch`, `selectedSubBranch`).

### Removed
- **`manifest.json`** – no longer created or used. All data lives in `.red-signer/signer.db`.
- **Legacy manifest reading/writing code** – fully replaced with database queries.
- **Default branch selection** – users must now explicitly choose a branch and sub‑branch on first use.

---

## [1.2.0] – 2026-05-30 (archived, manifest‑based version)

_This version used `manifest.json`; see previous changelog for details._

---

## [1.1.0] – 2026-05-15

## [1.0.0] – 2026-04-20