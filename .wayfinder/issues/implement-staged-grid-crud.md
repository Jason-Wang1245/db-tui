---
title: Implement staged grid CRUD
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - implement-paginated-table-browsing.md
  - define-record-browsing-and-staged-crud-semantics.md
---

## Question

Implement and verify per-tab insert, update, and delete staging; dirty-state presentation; cell editing and type-aware validation; explicit apply/revert; safe transactions and conflicts; generated/default/null values; refresh behavior; and close protection.

## Resolution

Completed staged grid CRUD on 2026-08-19.

### Per-tab staging and interaction

- Added independent insert, update, and delete change sets to every grid tab. Draft inserts stay pinned above fetched rows, updated cells carry visible markers, deletes remain dimmed tombstones, and text/symbol counts keep dirty state visible without relying on color.
- Added keyboard and clickable actions for inserting, editing, setting SQL `NULL`, setting `DEFAULT`, toggling deletes, applying, reverting one row, confirming a whole-tab revert, and opening a scrollable summary that includes off-page changes.
- Added a Unicode-safe raw cell editor with paste and horizontal cursor tracking. Submitting an empty editor stages `''`; clearing through the dedicated action stages `NULL`; generated and generated-identity columns remain read-only.
- Added immediate conservative validation for booleans, integers, numeric/float values, dates, times, timestamps, and UUIDs. JSON, arrays, enums, domains, ranges, and other text-representable values remain raw and PostgreSQL performs the authoritative cast.

### Safe apply, refresh, and conflict behavior

- Added immutable apply snapshots and a narrow mutation adapter. Deletes, updates, and inserts execute in one explicit pgx transaction with quoted identifiers and parameterized values; any statement, cast, constraint, permission, or optimistic-concurrency failure rolls back the whole batch and preserves every staged change.
- Updates and deletes match the original primary/qualifying unique-key values plus original `xmin`. Zero-row results become conflicts that show original, staged, and current database values; there is no force-overwrite path.
- Added server-authoritative PostgreSQL diagnostics with SQLSTATE, detail, hint, constraint, context, and affected-column focus when PostgreSQL identifies it, including raw/non-primitive cast failures.
- Successful apply clears staging only after commit and reloads the current page; Apply chosen from the dirty-refresh decision reloads page one. Generated/default/trigger values and changed ordering therefore come back from PostgreSQL.
- Cancellation rolls back and leaves the complete staged set retryable. A lost commit acknowledgement is treated separately from a known rollback: blind retry is blocked and the UI directs the user to reconnect, revert the local snapshot, and reload before restaging.

### Metadata, lifecycle, and verification

- Extended PostgreSQL metadata with relation/column INSERT, UPDATE, and DELETE privileges, generated/default/nullability details, and operation-specific read-only explanations. Grid CRUD remains limited to ordinary and partitioned tables with a safe identity and `xmin`; keyless tables, views, materialized views, foreign tables, and system catalogs stay read-only.
- Preserved staged work across paging, sorting, tab switches, connection loss, and reconnect. Dirty refresh offers Apply, Revert and refresh, or Cancel; dirty/running table close prompts explicitly describe discarded changes and cancelled work.
- Documented the editing controls and atomic/optimistic safety model in the README.
- `gofmt`, `git diff --check`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and `govulncheck ./...` pass. Grid coverage is 67.2%, with explicit reducer tests for draft values, validation, navigation persistence, summaries, refresh decisions, immutable/stale/cancelled applies, conflicts, uncertain commit recovery, mouse actions, and Unicode rendering; PostgreSQL builder tests cover parameter semantics, quoted optimistic predicates, metadata validation, server column focus, and commit classification.
- Testcontainers integration passes against PostgreSQL 14 and 18 for empty string versus `NULL`, defaults, generated columns, editable ordinary keys, raw JSON casts and structured errors, full-batch rollback, `xmin` conflict/current-row details, delete, cancellation rollback, and pool recovery. CGO-free macOS/Linux builds pass on `amd64` and `arm64`.

No new ticket is required. This closes the final blocker for [Harden navigation, reliability, and polish](harden-navigation-reliability-and-polish.md), whose existing integrated error-recovery scope includes packaged/manual commit-interruption acceptance.
