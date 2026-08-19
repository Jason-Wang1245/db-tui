---
title: Implement the SQL tab workspace
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - implement-secure-connection-management.md
  - implement-the-connected-workspace-shell.md
  - define-sql-tab-execution-behavior.md
---

## Question

Implement and verify multiple SQL editor tabs, the resolved execution and cancellation behavior, result and command-status presentation, error locations and messages, result navigation, transaction behavior, mouse support, and dirty-close protection without query history or persistence.

## Resolution

Completed the SQL tab workspace on 2026-08-19.

### Editor and execution behavior

- Added independent multiline Unicode SQL editors with two-space Tab insertion, keyboard and mouse selection, clipboard actions, undo/redo, vertical and horizontal viewports, and exact per-tab dirty state.
- Implemented selection-or-buffer execution, run-all, and exact rerun from immutable snapshots. Each tab permits one active run while other tabs may run concurrently and the active editor remains editable.
- Added clickable Execute, Run all, Rerun, and Cancel actions; `F5`/`Ctrl+Enter`, `F6`/`Ctrl+Shift+Enter`, `F7`, and `Ctrl+C` provide keyboard parity.
- Preserved session-local SQL and the latest result across reconnect without replay. Dirty/running close prompts explicitly describe discarding SQL and cancelling active runs; query history and persistence remain excluded.

### PostgreSQL execution and results

- Added isolated per-run pgx execution using PostgreSQL's simple-query protocol, submitting each immutable batch unchanged and preserving server-ordered row, command, notice, and error outputs.
- Added typed result cells with explicit `NULL` versus empty-string handling, safe terminal rendering, command tags and affected-row counts, elapsed timings, 100-row client pages, a full-value inspector, multi-row selection, copy actions, and horizontal/vertical result navigation.
- Enforced 10,000 rows per row result and a shared 64 MiB capture limit, with visible truncated/incomplete states. PostgreSQL errors show SQLSTATE, detail, hint, constraint, context, and positions mapped to the submitted snapshot.
- Cancellation retains completed/partial output and leaves exact rerun available. Every execution acquires its own connection, rolls back an open transaction with a warning, runs `DISCARD ALL`, and discards an unsafe connection so state cannot leak between runs.

### Application integration and verification

- Integrated SQL models with root-owned tab/request/cancellation state, stale-completion rejection, responsive content sizing, reconnect session replacement, contextual help/status, and modal-safe keyboard/mouse routing.
- Documented SQL controls, execution isolation, result limits, and session-only retention in the README.
- `gofmt`, `git diff --check`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and `govulncheck ./...` pass. SQL-tab coverage is 56.6%, with reducer, immutable-snapshot, concurrency, stale/cancel, selection/editing, result navigation, error, mouse, viewport, and Unicode-width paths covered.
- Testcontainers integration passes against PostgreSQL 14 and 18 for ordered multi-result batches, notices, `NULL` versus empty strings, structured server errors, transaction cleanup and session isolation, row truncation, cancellation, and pool recovery. CGO-free macOS/Linux builds pass on `amd64` and `arm64`.

No new ticket is required. This completes the SQL blocker for [Harden navigation, reliability, and polish](harden-navigation-reliability-and-polish.md); that ticket remains blocked on [Implement staged grid CRUD](implement-staged-grid-crud.md).
