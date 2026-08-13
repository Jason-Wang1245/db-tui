# db-tui

`db-tui` is a PostgreSQL management terminal UI under active development. The
initial bootstrap establishes the Go module, architectural boundaries, test
infrastructure, and continuous-integration gates that later feature slices use.

## Requirements

- Go 1.26.5
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

The executable currently presents a bootstrap launcher shell. Secure profile
management, the connected workspace, table browsing, staged CRUD, and SQL tabs
are implemented by the subsequent Wayfinder tickets.

This primarily personal-use repository intentionally has no project license.
