# Codesfer

[![Daily Pre-Release](https://github.com/GNITOAHC/codesfer/actions/workflows/prerelease.yml/badge.svg)](https://github.com/GNITOAHC/codesfer/actions/workflows/prerelease.yml)
[![Release](https://github.com/GNITOAHC/codesfer/actions/workflows/release.yml/badge.svg)](https://github.com/GNITOAHC/codesfer/actions/workflows/release.yml)

A CLI tool and self-hostable server for sharing code snippets and files with optional encryption.

## Installation

**Shell (Linux/macOS):**

```bash
curl -LsSf https://www.codesfer.io/install.sh | sh
```

**PowerShell (Windows):**

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://www.codesfer.io/install.ps1 | iex"
```

**Homebrew (macOS):**

```bash
brew tap gnitoahc/tap
brew install gnitoahc/tap/codesfer
```

**Go:**

```bash
go install github.com/gnitoahc/codesfer/cmd/codesfer@latest
```

**Binary:** Download pre-built releases from [GitHub releases](https://github.com/GNITOAHC/codesfer/releases).

**From source** (requires Go 1.24+):

```bash
git clone https://github.com/GNITOAHC/codesfer.git
cd codesfer
make all # Builds 'codesfer' (CLI) and 'codeserver' (Server) to ./build/
```

## CLI Usage

### Auth & Account

- `codesfer auth` (Register, Login, Logout)
- `codesfer account` (View profile)

### Share Files

- **Push**: `codesfer push <file> [-k alias] [-d desc] [--pass password]`
- **Pull**: `codesfer pull <code|alias> [-o out_dir] [--pass password]`
- **Manage**: `codesfer list` / `remove <code|alias>`

`codesfer list` will also show URL which can be downloaded with direct `wget` or `curl`

### Config

- `codesfer config set|get <key> [value]`

## Self-hosting

Run `codeserver serve --port 3000`.

### Self-hosting Configuration (.env)

Run `codeserver init` to generate a `.env` file.

- `DB_SOURCE`: Auth DB path.
- `INDEX_DB_SOURCE`: File index path.
- `OBJECT_BACKEND_DRIVER`: `sqlite` (local) or `r2` (Cloudflare).
- `OBJECT_STORAGE_SOURCE`: Path for SQLite storage.
- **R2 Config**: `CF_ACCOUNT_ID`, `CF_ACCESS_KEY`, `CF_SECRET_ACCESS_KEY`, `CF_BUCKET`.
