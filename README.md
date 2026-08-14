# db-tui

`db-tui` is a PostgreSQL management terminal UI under active development. It
currently includes secure local connection-profile management and a PostgreSQL
connection/test flow.

## Requirements

- Go 1.26.6
- PostgreSQL 14 or newer for integration testing
- Docker for Testcontainers-backed integration tests

## Development

```sh
make check
make integration
make build
./db-tui --version
```

`make check` runs formatting verification, `go vet`, unit and architecture
tests, the race detector, and `govulncheck`. Integration tests use PostgreSQL
18 by default; set `TEST_POSTGRES_VERSION` to exercise another supported major.

## Current scope

The connection launcher supports:

- Field-based PostgreSQL profiles and creation-time connection URL import
- Validation, cancellable 10-second connection tests, and actionable errors
- Save, edit, quick-connect, and confirmed deletion workflows
- Atomic non-secret profile storage with restrictive filesystem permissions
- Password storage in the operating-system keychain or session-only fallback

From the profile list, use `n` to create, `e` to edit, `t` to test, `Enter` to
connect, and `d` to delete. In the form, use `Ctrl+U` to import the masked URL,
`Ctrl+P` to toggle keychain storage, `Ctrl+S` to save, `Ctrl+T` to test, and
`Ctrl+Enter` to connect. `Esc` cancels active work or discards a draft.

The connected workspace, table browsing, staged CRUD, and SQL tabs are handled
by subsequent implementation slices.

## Profiles and passwords

Profiles are stored in `profiles.json` below the OS user-config directory. The
directory is mode `0700`, the file is mode `0600`, and writes use an atomic
temporary-file rename. The JSON contains canonical connection fields and a
stable UUID; it never contains a password or the imported connection URL.

On macOS, saved passwords use the login Keychain. On Linux, they use Secret
Service and require an available, unlocked collection (normally `login`). If
the keychain is unavailable, the profile can still be saved and the password
is clearly kept for the current process only. Plaintext persistence is never a
fallback. Turning **Save password** off or deleting a profile removes its
keychain entry.

This primarily personal-use repository intentionally has no project license.
