---
title: Implement the connected workspace shell
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - bootstrap-the-project-and-ci.md
  - prototype-the-end-to-end-interaction-model.md
---

## Question

Implement and verify the connected workspace, expandable schema/table sidebar, focus system, mixed grid/SQL tab bar, tab lifecycle, responsive layout, help and status surfaces, keyboard navigation, and mouse interactions from the accepted prototype.

## Resolution

Completed the connected workspace shell on 2026-08-15.

### Workspace interaction

- Replaced the connected placeholder with a profile/database/version header, lazy expandable object navigator, ordered mixed table/SQL tab strip, active content surface, contextual action row, persistent status row, and built-in help.
- Added one visible focus path across navigator, tab strip, and active content. Keyboard and mouse actions share the same open, activate, close, refresh, reconnect, disconnect, and help transitions.
- Implemented table-tab deduplication, session-local `SQL 1`, `SQL 2`, … labels, stable ordering, previous/next activation, focus restoration, ANSI/grapheme-safe truncation, active-tab scrolling, and explicit mouse close targets.
- Added clean immediate close plus safe-default confirmation for dirty/running tab close, disconnect, and quit. Confirming a running close emits cancellation for that tab's active operation; one leave modal summarizes all hazardous tabs.

### Responsive and asynchronous behavior

- Added wide split-pane, compact drawer, and single-pane layouts at the accepted breakpoints while preserving state and focus identity; the existing root minimum-size guard keeps all workspace state intact below `48×12`.
- Added request-identified schema, relation, health-check, and reconnect operations with cancellation registry integration and stale-completion rejection.
- Added periodic connection health checks, a persistent lost-connection surface, explicit retry/disconnect paths, generation-safe timers, and saved-profile reconnect that replaces the live session while preserving tabs and never replaying edits or SQL.
- Preserved sanitized structured PostgreSQL errors—including SQLSTATE, detail, hint, and context—on the owning workspace surface.

### PostgreSQL catalog adapter

- Extended the live pgx session with lazy `Schemas` and per-schema `Relations` capabilities using parameterized catalog queries, deterministic ordering, relation-kind mapping, schema visibility filtering, and `USAGE`/`SELECT` privilege reporting.
- Kept pgx and catalog SQL inside the PostgreSQL adapter; the workspace consumes only its narrow feature-owned interface, and the app root retains ownership of the live session and asynchronous commands.
- Documented the connected controls and the intentional table-data/SQL-execution placeholders in the README.

### Verification

- `gofmt`, `git diff --check`, `go vet ./...`, `go test ./...`, and `go test -race ./...` pass; workspace reducer/rendering coverage is 69.8% with explicit critical-transition tests.
- Reducer and app lifecycle tests cover lazy catalog loading, stale results, focus breakpoints, keyboard/mouse parity, tab order/deduplication, safe close/leave behavior, cancellation, Unicode/ANSI width, connection loss, stale health timers, reconnect session replacement, and no-replay tab preservation.
- PostgreSQL Testcontainers integration tests pass against majors 14 and 18 with real schema/relation discovery and privilege metadata.
- `govulncheck ./...` reports no reachable known vulnerabilities, and CGO-free builds pass for macOS/Linux on `amd64` and `arm64`.

No new ticket is required. This completes the shared shell blocker and makes both [Implement paginated table browsing](implement-paginated-table-browsing.md) and [Implement the SQL tab workspace](implement-the-sql-tab-workspace.md) available to pick up.
