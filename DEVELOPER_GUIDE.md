# Obsidian RED-Feather — Developer Guide

> Audience: developers continuing work on `obsidian-red-signer` and the surrounding
> contributor-onboarding tooling.
> Source of truth is the code. (`CHANGELOG.md` is known to be stale — ignore it.)

---

## 1. What this tool is

A hybrid Obsidian plugin that cryptographically signs vault notes so a `red-engine`
node can later display them as **verified**. Two halves:

- **TypeScript UI** ([src/main.ts](src/main.ts)) — the Obsidian plugin: ribbon icon,
  command palette entries, context-menu item, status-bar indicator, and the branch
  selection modal. It does **no crypto itself**; it shells out to the Go binary via
  `execFile`.
- **Go binary** ([cmd/red-feather/main.go](cmd/red-feather/main.go)) — the crypto +
  database engine. Version `2.2.0`. Loads/creates the Ed25519 identity, signs files,
  and manages the per-vault `signer.db`.

```
Obsidian (src/main.ts)
   │  execFile(binary, ["--db", signer.db, <file> | --flags])
   ▼
red-feather (Go)
   │  ed25519.Sign(privkey, sha256(fileBytes))
   ├── ~/.red-feather/maintainer.key   (0600, the identity)
   └── <vault>/.red-feather/signer.db  (per-vault signatures + metadata)
```

### Identity & trust model

- The private key is at `~/.red-feather/maintainer.key` (0600), generated on first use
  ([main.go:349-379](cmd/red-feather/main.go#L349-L379)). It is the author's permanent
  identity across every vault. `~/.red-feather/README.md` warns the user never to
  delete it ([src/main.ts:303-328](src/main.ts#L303-L328)).
- The **first** key to sign/init a vault becomes its permanent `branch_author`
  ([main.go:578-586](cmd/red-feather/main.go#L578-L586)). Only that key may change the
  vault's branch classification afterward
  ([main.go:504-507](cmd/red-feather/main.go#L504-L507)).
- Signing computes `sha256(fileBytes)` and signs the **hash bytes**
  ([main.go:555-558](cmd/red-feather/main.go#L555-L558)). `red-engine` verifies that
  exact form. Keep the two in lockstep if you ever change the signed payload.

---

## 2. Build & run

```bash
# Go binary (module lives in cmd/red-feather — there is no top-level Go module)
cd cmd/red-feather
go build -o red-feather-linux-x64  # name must match the platform switch in src/main.ts
go vet ./...                       # run from inside cmd/red-feather, not the repo root

# Plugin bundle
cd src
npm install
npm run build                      # produces main.js next to manifest.json
```

The plugin picks the binary by platform/arch
([src/main.ts:208-222](src/main.ts#L208-L222)): `red-feather-linux-x64/arm64`,
`red-feather-macos-x64/arm64`, `red-feather-windows-x64.exe`. On non-Windows it `chmod 0755`s the
binary if needed. Ship one binary per platform alongside `main.js` + `manifest.json`.

### Binary CLI (the whole contract the UI depends on)

| Invocation | Effect |
|---|---|
| `--version` | print `2.2.0` |
| `--print-pubkey` | print this machine's hex public key (no `--db` needed) |
| `--db <p> --init --branch B [--sub-branch S]` | create/seed `signer.db`, set author |
| `--db <p> --set-branch --branch B [--sub-branch S]` | author-only reclassification |
| `--db <p> --get-branch` | print `{branch,sub_branch,branch_author}` JSON |
| `--db <p> --check-status <file>` | print `signed` / `unsigned` / `modified` |
| `--db <p> <file>` | **sign** the file (upsert into `files`) |

Adding a flag means adding a parallel `execFilePromise` call in `src/main.ts`. Keep the
binary's stdout contract stable — the UI parses it (`JSON.parse(stdout)` for
`--get-branch`, `stdout.trim()` for status).

---

## 3. Database schema (signer.db)

Migrations are an ordered, append-only list, each in its own transaction, recorded in
`schema_migrations` ([main.go:40-152](cmd/red-feather/main.go#L40-L152)). Current head =
4. **Never reorder or renumber existing migrations — only append.**

| Table | Purpose |
|---|---|
| `files` | `path` PK, `file_hash`, `public_key`, `signature`, `updated_at`, `sign_count`. Indexed on `public_key` (speeds red-engine reloads) and `updated_at`. |
| `vault_metadata` | Singleton (`CHECK(id = 1)`): `branch`, `sub_branch`, `branch_author`, `initialized_at`, `updated_at`. Replaced the old `metadata` KV bag in migration 3. |
| `schema_migrations` | `version` PK, `applied_at`, `description`. |

`upsertFileEntry` bumps `sign_count` on every re-sign via `ON CONFLICT(path) DO UPDATE`
([main.go:272-293](cmd/red-feather/main.go#L272-L293)). A legacy pre-migration DB
(`files` present, nothing recorded) is adopted as version 1 so later migrations
transform it forward instead of re-creating tables
([main.go:199-206](cmd/red-feather/main.go#L199-L206)).

To add schema: append a `migration{version: 5, …}` with an idempotent `apply`. Guard
`ALTER TABLE … ADD COLUMN` against `duplicate column name` (see migration 4).

---

## 4. Current UX flow (and its limits)

1. User opens a note, clicks the ribbon / runs "Sign current note" / uses the context
   menu.
2. If no branch is set, the **SignModal** forces branch + sub-branch selection from a
   fixed taxonomy ([src/main.ts:10-17](src/main.ts#L10-L17)) before the Sign button
   enables.
3. `signFile` shells out `--db <signer.db> <fullPath>`; on success it reloads branch
   info and refreshes the status bar
   ([src/main.ts:476-513](src/main.ts#L476-L513)).
4. The status bar shows `✓ Signed` / `⚠ Modified` / `Unsigned` for the active file via
   `--check-status` ([src/main.ts:429-463](src/main.ts#L429-L463)).

**The structural limitation: everything is one-file-at-a-time.** There is no batch
operation, no "sign everything that changed", and no notion of *which* files are stale
across the whole vault at once. The status bar only reflects the *active* file.

---

## 5. Features intentionally left off (the backlog)

These are the deliberately-deferred pieces. Each entry says *what*, *why it's missing*,
and *how to implement it against the existing code*.

### 5.1 Sign all modified files in one click

**Why missing:** signing is wired to the single active `TFile`; there's no vault scan.

**How to add it:**
- **Binary:** add a `--sign-all` mode that, given `--db`, walks the vault root
  (`filepath.Dir(filepath.Dir(dbPath))`), and for every `.md` file whose current
  `sha256` differs from the stored `files.file_hash` (reuse `checkFileStatus`), runs
  the existing sign path (`upsertFileEntry`). Emit a JSON summary
  (`{signed: [...], skipped: n}`) on stdout so the UI can report counts. Keep it one
  transaction-per-file so a mid-run failure doesn't corrupt the set.
- **UI:** add a command `"Sign all modified notes"` and a ribbon action that calls the
  new flag and surfaces the summary in a `Notice`. You can pre-compute the modified set
  client-side too (you already have `checkFileStatus` per file) and pass an explicit
  list, but a single binary call is faster and avoids N process spawns.
- **Guard:** respect the branch-author lock — re-signing is fine for any trusted
  contributor, but `--sign-all` must not silently create a vault author; only sign
  files, and only set `branch_author` if it's genuinely the first signature, exactly
  like the single-file path.

### 5.2 More security / hardening

Current gaps and the fixes:
- **Key at rest is unencrypted.** `maintainer.key` is raw 0600 bytes. Add optional
  passphrase encryption (e.g. scrypt + secretbox) with the binary prompting (or the UI
  passing) a passphrase. Back-compat: detect the unencrypted form by length
  (`ed25519.PrivateKeySize`).
- **No revocation / rotation story.** If a key leaks there's no way to retire it beyond
  the node operator revoking on each engine. Add a key-rotation record signed by the
  old key, so engines can migrate a vault's `branch_author` forward.
- **Signed payload is just the hash.** Consider signing a small envelope
  (`hash | path | branch`) so a signature can't be lifted from one file's record onto
  another. This is a coordinated change with `red-engine`'s verify path
  ([store.go:300-336](../red-engine/internal/store/store.go#L300-L336)) — bump the
  signer version and have the engine accept both forms during transition.
- **No tamper log.** `sign_count` exists but isn't surfaced; expose it so reviewers can
  spot churn.

### 5.3 Website self-signup for contributors (public key + display name)

Goal: let a would-be contributor register their public key and a chosen (possibly
pseudonymous) username **through the node's website**, so the operator can approve them
into `trusted_authors` without out-of-band coordination.

**Design — build this as a self-contained onboarding source file**, e.g.
`red-engine/internal/router/onboard_web.go`:
- `GET /-/join` → a small public form (key + desired display name). The form should
  show the applicant how to get their key (`red-feather --print-pubkey`).
- `POST /-/join` → validate the key is 64-char hex (mirror the check in
  [contributors.go:54-57](../red-engine/internal/router/contributors.go#L54-L57)),
  then insert into a **pending** queue, not directly into `trusted_authors`. Reuse the
  `imported_from` column with a value like `web_pending` and keep `revoked = 1` (or add
  a `pending` status) so an unapproved applicant is never trusted.
- Operator approves from the admin panel → flip to trusted (`revoked = 0`,
  `imported_from = 'web'`). This reuses `addContributorToDB`'s upsert
  ([contributors.go:66-73](../red-engine/internal/router/contributors.go#L66-L73)).
- **Abuse controls:** rate-limit the endpoint, cap pending rows, and never auto-trust.
  A "fake username" is fine (pseudonymity is a feature) — the *key* is the real
  identity; the name is just a label.

### 5.4 Discord / external-platform onboarding (the second, separate file)

Same destination (`trusted_authors`), different intake. Per the original intent, keep
this in its **own file** so the two onboarding paths never entangle, e.g.
`red-engine/internal/router/onboard_discord.go`:
- A bot (or webhook handler) collects `public_key` + Discord handle, hits a
  token-protected `POST /-/admin/contributors/intake { source: "discord", handle,
  public_key }`.
- Stamp `imported_from = 'discord:<handle>'` so provenance is visible in the admin
  list and the operator can filter/trust by source
  (`idx_trusted_authors_imported_from` already exists for this).
- Because each onboarding source writes the **same** `trusted_authors` table keyed by
  `public_key`, the same person arriving via web *and* Discord collapses to one row —
  no duplicate identities. The intake file is just a thin adapter; the
  add/dedupe/revoke logic stays shared.

> Pattern: **one trust table, many intake adapters.** Web form, Discord bot, and
> peer-import (next) are all separate files that funnel into the same upsert. This is
> the clean extension point — add `onboard_<x>.go`, don't fork the table.

### 5.5 Cross-node author import without duplicates (verified-everywhere authors)

Goal: when node B imports content/authors from node A, a verified author should be
recognised as verified on **every** node — as a single identity — while the operator
retains the right to **un-verify** an imported author on *their* node only.

**Why it's already half-built:** `trusted_authors` is keyed by `public_key PRIMARY
KEY`, and carries `imported_from`, `imported_at`, `revoked`, `revoked_at`, `signature`
([registry.go:91-102](../red-engine/internal/registry/registry.go#L91-L102)). That
schema is exactly what de-duplicated, revocable, provenance-tagged import needs.

**How to implement:**
- **Publish side (node A):** add a public `GET /-/contributors` returning
  `[{public_key, name, signature?}]` for non-revoked authors (mirror the shape of
  `listContributors`, but unauthenticated and read-only). Optionally sign the list with
  the node key so importers can verify it came from A.
- **Import side (node B):** `POST /-/admin/contributors/import { peer_url }` fetches
  A's `/-/contributors` and upserts each with
  `ON CONFLICT(public_key) DO UPDATE SET name = excluded.name, imported_from =
  excluded.imported_from` — **the public-key PK guarantees no duplicates**. Stamp
  `imported_from = peer_url` so the operator sees where each author came from.
- **Operator override:** the operator removes an imported author with the **existing**
  `revokeContributor` ([contributors.go:84-120](../red-engine/internal/router/contributors.go#L84-L120))
  — a soft delete (`revoked = 1`, `revoked_at = now`). Because the store only loads
  `WHERE revoked = 0`, a revoked import instantly stops verifying on *this* node while
  remaining verified elsewhere. Re-importing won't resurrect a locally-revoked author
  unless you choose to clear `revoked` on conflict — **don't**; respect the local
  operator's decision (i.e. on conflict, update `name`/provenance but leave `revoked`
  alone).
- **Trust caveat to document for operators:** importing authors means trusting node A's
  curation. The optional signed-list step (and the unused `trusted_authors.signature`
  column) is where you'd add per-author vouching so import isn't blind trust.

### 5.6 And beyond

- **Author display directory** on the node (`/-/authors`) listing verified
  contributors, their key, and which content they signed — the read model over
  `trusted_authors` + signer `files`.
- **Per-file author attribution in `/-/peers`** so the network can answer "who verified
  this article" before you pull it.
- **Vault-wide re-verify command** in the plugin that flags every file whose stored
  hash no longer matches (the batch sibling of the status bar).
- **Multi-author vaults**: today branch classification is single-author-locked;
  co-maintainer support would need a `vault_authors` table and a policy for who may
  reclassify.

---

## 6. How the signer and engine fit together (the contract)

```
signer.db.files: path, file_hash, public_key, signature
        │  (red-engine walks every .red-feather/signer.db under data/)
        ▼
red-engine store.loadSecurityData → allSignatures[path]
        │  cross-checked against registry.trusted_authors WHERE revoked = 0
        ▼
store.processArticle → VerificationState: verified | tampered | untrusted | invalid_sig
```

Three things must stay consistent across the two repos or verification silently breaks:
1. **The signed payload.** Signer signs `sha256(content)` bytes; engine verifies that
   form (among the three it tries).
2. **The relative path.** Signer stores paths relative to the **vault root**
   (`filepath.Dir(filepath.Dir(dbPath))`,
   [main.go:563-570](cmd/red-feather/main.go#L563-L570)); the engine reconstructs the
   same key relative to its `data/` dir
   ([store.go:243-268](../red-engine/internal/store/store.go#L243-L268)). If you change
   path handling on one side, change both.
3. **The public key encoding.** Lower-case hex, 64 chars. The engine lower-cases keys
   when building the trust map; keep the signer emitting hex.

---

## 7. Conventions & gotchas

- **The Go module is `cmd/red-feather`**, not the repo root. Run `go build`/`go vet`
  from inside it.
- **Binary filenames are load-bearing** — they must match the platform switch in
  `src/main.ts`.
- **Migrations are append-only.** Adopt-legacy logic assumes migration 1 == the
  original `files`+`metadata` schema.
- **`branch_author` is set-once.** First signer wins; only they reclassify. Don't add a
  path that silently overwrites it.
- **Keep stdout machine-parseable** for `--get-branch` / `--check-status`; the UI
  depends on it.
- **One trust table, many intake adapters** — web, Discord, and peer-import all upsert
  into `trusted_authors` keyed by `public_key`. That key is what makes "verified
  everywhere, one identity, locally revocable" work without duplicates.
```
