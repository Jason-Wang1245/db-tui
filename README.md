# db-tui

`db-tui` is a PostgreSQL management terminal UI under active development. It
currently includes secure connection management, the connected workspace, and
paginated table browsing; staged grid CRUD and SQL execution are the next
implementation slices.

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

The connected workspace provides:

- Lazy, expandable schema/relation navigation with PostgreSQL privilege hints
- One ordered strip for deduplicated table tabs and session-local SQL tabs
- Typed table metadata and 100-row server-side pages, with deterministic keyset
  pagination for keyed relations and labelled best-effort browsing otherwise
- Per-column sorting, refresh, responsive column sizing, horizontal navigation,
  safe PostgreSQL value formatting, and recoverable detailed database errors
- Keyboard and mouse parity, contextual help, safe close/leave confirmation,
  stable status feedback, and responsive wide/drawer/single-pane layouts
- Connection health checks and explicit reconnect/disconnect behavior that
  preserves open tabs without replaying edits or SQL

Use `Tab` / `Shift+Tab` to move focus, `Space` to expand schemas, `Enter` to
open a relation, `[` / `]` to switch tabs, and `n` / `x` on the tab strip to
create or close tabs. `?` opens contextual help and `Ctrl+D` disconnects.

In a table grid, use arrows or `h`/`j`/`k`/`l` to move between cells, `n`/`p`
or Page Down/Page Up to change pages, `s` to cycle the selected column through
ascending, descending, and default order, and `r` to refresh from page one.
Clicking selects a cell; the wheel moves by rows and Shift+wheel moves by
columns. `Ctrl+C` cancels an active load, which can then be rerun normally.

SQL tabs intentionally show a shell placeholder until their execution slice is
implemented. Table tabs are browse-only until staged CRUD is implemented.

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
