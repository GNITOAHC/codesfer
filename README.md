# Codesfer

A CLI tool and self-hostable server for sharing code snippets and files with optional encryption.

## Installation

Requires Go 1.24+.

```bash
make all # Builds 'codesfer' (CLI) and 'codeserver' (Server) to ./build/
```

Or install from [releases](https://github.com/GNITOAHC/codesfer/releases).

## CLI Usage

### Auth & Account

- `codesfer register` / `login` / `logout`
- `codesfer account` (View profile)

### Share Files

- **Push**: `codesfer push <file> [-k alias] [-d desc] [--pass password]`
- **Pull**: `codesfer pull <code|alias> [-o out_dir] [--pass password]`
- **Manage**: `codesfer list` / `remove <code|alias>`

### Config

- `codesfer config set|get <key> [value]`

## Self-hosting

Run `./build/codeserver -port 3000`.

### Configuration (.env)

- `DB_SOURCE`: Auth DB path.
- `INDEX_DB_SOURCE`: File index path.
- `OBJECT_BACKEND_DRIVER`: `sqlite` (local) or `r2` (Cloudflare).
- `OBJECT_STORAGE_SOURCE`: Path for SQLite storage.
- **R2 Config**: `CF_ACCOUNT_ID`, `CF_ACCESS_KEY`, `CF_SECRET_ACCESS_KEY`, `CF_BUCKET`.
