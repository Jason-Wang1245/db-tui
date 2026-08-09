---
title: Define record browsing and staged CRUD semantics
label: wayfinder:grilling
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by: []
---

## Question

What ordering, pagination, row-identity, insert/update/delete staging, validation, transaction, conflict, refresh, null/default, and unsupported-table rules keep grid browsing predictable and mutations safe across real PostgreSQL tables?

## Resolution

Accepted the following record-browsing and staged-CRUD contract on 2026-08-09.

### Ordering and pagination

- A writable relation's default order is its chosen row-identity key in ascending order.
- A user may sort by one visible column at a time. The identity columns are appended as invisible tie-breakers so the effective order remains unique; changing the sort returns to the first page.
- Keyed relations use cursor/keyset pagination with 100 rows per page and previous/next navigation. V1 has no arbitrary page jump or automatic total-row count.
- Refresh preserves the active sort and returns to the first page.
- Relations without a usable unique key remain browseable in a clearly labelled read-only, best-effort mode. They use `OFFSET` pagination with the most deterministic available ordering and warn that concurrent changes can cause repeated or skipped rows.

### Row identity and relation editability

- Prefer the primary key. Otherwise choose the shortest qualifying unique key, using a deterministic tie-break when several have the same width.
- A qualifying unique key is non-partial, non-expression, composed only of ordinary columns, and has every component declared `NOT NULL`. Composite keys are supported; nullable unique keys do not qualify.
- Do not use `ctid` or whole-row matching as a mutation identity fallback.
- Ordinary primary-key and qualifying unique-key columns are editable. Mutations locate an edited row with its original key values.
- Browse ordinary and partitioned tables, views, materialized views, and foreign tables when permissions allow. Grid CRUD is limited to ordinary or partitioned tables with a usable identity key; other relations and system catalogs are read-only even when PostgreSQL could technically update them.
- The UI explains why a relation or operation is read-only. Known privileges may disable controls proactively, but PostgreSQL remains authoritative if permissions change.

### Per-tab staging and presentation

- Every grid tab owns an independent change set. Changes persist across page and sort navigation, tab switches, and connection loss/reconnect, with an always-visible dirty count.
- Inserts appear as pinned draft rows above fetched data, updated cells are highlighted inline, and deletes remain as dimmed tombstones until applied or reverted.
- A change-summary view lists all staged inserts, updates, and deletes, including rows not on the current page.
- A single row can be reverted directly. Reverting the entire tab requires confirmation.
- Refresh never silently discards or rebases pending work. A dirty-tab refresh offers **Apply**, **Revert and refresh**, or **Cancel**; closing a dirty tab receives the equivalent safe confirmation.

### Cell values and type handling

- **Clear field** explicitly stages SQL `NULL`.
- Opening a text-like cell editor and submitting no characters stages the empty string `''`, preserving its distinction from `NULL`.
- **Set default** explicitly stages SQL `DEFAULT`.
- New rows begin with insertable columns in the default state. Required columns without defaults are highlighted before apply.
- PostgreSQL-generated columns and generated identity columns are read-only and are populated by the post-commit reload. Ordinary primary/unique-key columns remain editable.
- Common scalar types use typed editors and immediate format validation. JSON, arrays, enums, domains, ranges, and other text-representable types use a raw-text editor with parameterized values and PostgreSQL performing the final cast.
- Truly opaque or non-writable columns are read-only individually rather than making the entire row read-only. Insert is unavailable when a required non-writable column lacks a default; update and delete may still be available.

### Validation, apply, and conflicts

- The client catches known required-value and primitive-format errors before confirmation. PostgreSQL is authoritative for casts, constraints, triggers, and permissions.
- PostgreSQL failures show the server message prominently and include detail, hint, SQLSTATE, and constraint name when present. The UI marks and focuses the affected row or cell when it can identify one; this explicitly includes raw/non-primitive value errors.
- **Apply** first presents insert/update/delete counts and the change summary. After confirmation, the entire tab's change set runs in one explicit transaction.
- Any validation, cast, permission, constraint, statement, or concurrency failure rolls back the complete batch. Nothing is presented as partially applied, and the staged change set remains available for correction or retry.
- Updates and deletes use optimistic concurrency predicates containing the original identity key and original PostgreSQL `xmin`. A zero-row result is a conflict and rolls back the batch.
- Conflict details show the original, staged, and current database values. V1 has no force-overwrite path; the user refreshes and restages deliberately.
- Staged state is cleared only after a confirmed commit. A successful apply reloads the current page so server defaults, triggers, deletes, and changed ordering are reflected.

No new ticket is required. Implementation belongs to [Implement paginated table browsing](implement-paginated-table-browsing.md) and [Implement staged grid CRUD](implement-staged-grid-crud.md); real-schema edge cases remain in the map's existing fog until the first end-to-end browsing slice makes them concrete.
