---
title: Implement paginated table browsing
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - implement-secure-connection-management.md
  - implement-the-connected-workspace-shell.md
  - define-record-browsing-and-staged-crud-semantics.md
---

## Question

Implement and verify schema discovery, table opening, metadata loading, deterministic server-side pagination, refresh, column sizing and horizontal navigation, PostgreSQL value formatting, loading and empty states, cancellation, and recoverable browsing errors.

## Resolution

Completed paginated table browsing on 2026-08-19.

### Browsing behavior

- Replaced table placeholders with independently stateful grid tabs that load typed relation metadata and the first 100-row page asynchronously.
- Added primary-key-first identity discovery with deterministic fallback to the shortest qualifying non-partial, non-expression, `NOT NULL` unique key.
- Implemented opaque, parameterized keyset cursors for keyed relations, including nullable selected-column sorting plus identity tie-breakers and forward/backward paging. Keyless relations use clearly labelled, read-only, best-effort `OFFSET` paging with deterministic available ordering.
- Refresh preserves the selected sort and returns to page one. Sorting cycles the selected sortable column through ascending, descending, and default identity order.

### Grid interaction and PostgreSQL adapter

- Added row/cell selection, viewport tracking, content-aware column widths, horizontal virtualization, arrows and `h`/`j`/`k`/`l`, Home/End, Page Up/Page Down, click selection, row wheel scrolling, and Shift+wheel column movement.
- Kept raw PostgreSQL values separate from safe single-line display values, with explicit `NULL`, bytea hex, date/timestamp, JSON, Unicode, and terminal-control handling. Rows retain original identity values and `xmin` for the staged-CRUD slice.
- Used `pgx.Identifier.Sanitize` for all identifiers and parameters for every cursor value. Ordinary/partitioned tables, views, materialized views, and foreign tables are browsable when privileges permit.
- Added request/tab/workspace identities, per-tab cancellation, stale-result rejection, retryable load failures, loading/empty states, and visible sanitized PostgreSQL messages with SQLSTATE, detail, hint, constraint, and context.

### Application integration and verification

- Integrated grid models with the root-owned workspace lifecycle, responsive content rectangles, tab close/cancel handling, reconnect session replacement, and keyboard/mouse routing without coupling the workspace shell to PostgreSQL.
- Documented table controls and current browse-only scope in the README.
- `gofmt`, `git diff --check`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and `govulncheck ./...` pass. Grid coverage is 78.0%; reducer, stale/cancel, sorting/pagination, mouse/keyboard, error, Unicode-width, and app lifecycle paths are covered.
- Testcontainers integration passes against PostgreSQL 14 and 18 for metadata, primary identity, forward/backward keyset pages, nullable sorting, keyless fallback, quoted identifiers, views, value formatting, and cursor behavior. CGO-free macOS/Linux builds pass on `amd64` and `arm64`.

No new ticket is required. This unblocks [Implement staged grid CRUD](implement-staged-grid-crud.md); broader integrated polish remains in [Harden navigation, reliability, and polish](harden-navigation-reliability-and-polish.md).
