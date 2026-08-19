---
title: Database TUI from concept to release
label: wayfinder:map
status: open
assignee:
---

## Destination

Ship a polished PostgreSQL management TUI built with the Bubble Tea stack: secure saved connections, expandable schemas and tables, multi-tab paginated data grids with staged CRUD, multi-tab custom SQL, crisp keyboard and mouse navigation, and distributable macOS/Linux binaries in a GitHub release.

## Notes

- Execution is explicitly part of this map: after prerequisite decisions close, `wayfinder:task` tickets implement and verify the product through its first release.
- Consult the Wayfinder skill and the active repository instructions before working a ticket.
- PostgreSQL is the only database engine. Connections use host/port credentials or a connection URL, with passwords stored in the operating-system keychain.
- The interaction standard is lazygit-inspired speed, clarity, and keyboard flow, with complete mouse support; it is not a visual clone.
- Grid mutations are staged per tab and require explicit apply or revert.
- Query and grid tabs may coexist in any combination. Query history and saved queries are excluded.
- Research should prefer official Go, PostgreSQL, Charmbracelet, and release-tool documentation. The dedicated `/research` helper is unavailable in this environment, so research tickets may use a standard research agent.
- Preserve scope boundaries unless the destination itself is deliberately redrawn.

## Decisions so far

<!-- Closed-ticket links and one-line gists go here. -->

- [Research the Go, PostgreSQL, and TUI foundations](research-the-go-postgresql-tui-foundations.md) — Use a CGO-free Go 1.26/Bubble Tea v2/pgx v5 stack with a custom editable grid, OS keyrings, layered tests, and GoReleaser-built macOS/Linux artifacts.
- [Prototype the end-to-end interaction model](prototype-the-end-to-end-interaction-model.md) — Use a root-owned focus model, layered keyboard/mouse interactions, stateful mixed tabs, safe modals, stable feedback, and progressive single-pane degradation.
- [Define connection profile behavior](define-connection-profile-behavior.md) — Use field-based profiles with creation-time URL import, explicit test/save/connect actions, per-profile keychain control, safe reconnect, and atomic versioned non-secret storage.
- [Define record browsing and staged CRUD semantics](define-record-browsing-and-staged-crud-semantics.md) — Use deterministic keyset browsing for keyed relations, best-effort read-only keyless browsing, explicit per-tab staging, atomic `xmin`-guarded applies, and conservative relation/type editability.
- [Define SQL tab execution behavior](define-sql-tab-execution-behavior.md) — Use explicit selection-or-buffer execution, unchanged server-parsed batches, isolated per-run sessions, bounded ordered results, deliberate cancellation/reruns, and safe dirty-tab handling.
- [Define the release quality bar](define-the-release-quality-bar.md) — Require four native macOS/Linux artifacts, PostgreSQL 14-through-current compatibility, layered automated/manual/security/accessibility/performance gates, verified unsigned releases, and immutable gated publication.
- [Choose the application architecture](choose-the-application-architecture.md) — Use a single-binary feature-oriented modular monolith with root-owned Bubble Tea state, typed request/result commands, narrow service interfaces, structured cancellation, sanitized errors, and enforced dependency seams.
- [Bootstrap the project and CI](bootstrap-the-project-and-ci.md) — Established the pinned Go 1.26 modular-monolith skeleton, executable and service seams, layered tests, PostgreSQL Testcontainers matrix, native smoke workflow, and verified four-target release snapshots.
- [Implement secure connection management](implement-secure-connection-management.md) — Added the responsive profile launcher, safe URL import and validation, atomic non-secret persistence, UUID-keyed keychain/session password lifecycle, cancellable PostgreSQL testing, and sanitized actionable connection errors.
- [Implement the connected workspace shell](implement-the-connected-workspace-shell.md) — Added lazy schema/relation navigation, responsive root-owned focus and mouse behavior, safe mixed-tab lifecycle, health-aware reconnect/disconnect, and PostgreSQL catalog/privilege discovery.
- [Implement paginated table browsing](implement-paginated-table-browsing.md) — Added typed grid tabs with safe PostgreSQL metadata/value handling, deterministic keyset paging, labelled best-effort keyless paging, responsive keyboard/mouse navigation, and recoverable cancellable loads.
- [Implement the SQL tab workspace](implement-the-sql-tab-workspace.md) — Added independent multiline SQL tabs with immutable batch execution, isolated cancellable sessions, bounded ordered results, structured PostgreSQL errors, and safe session-local lifecycle handling.
- [Implement staged grid CRUD](implement-staged-grid-crud.md) — Added per-tab insert/update/delete staging, explicit empty/NULL/default values, atomic parameterized applies, `xmin` conflict diagnostics, generated-value reloads, and safe dirty lifecycle handling.

## Not yet specified

- Release hardening discovered by testing packaged binaries on representative macOS and Linux environments.

## Out of scope

- Database engines other than PostgreSQL.
- Windows support.
- SSH tunneling and advanced per-profile SSL configuration.
- Query history and saved-query management.
- Cross-execution SQL transactions or other persistent SQL-session state.
- SQL IDE features such as syntax highlighting, autocomplete, formatting, and current-statement parsing; result-file export.
- Apple Developer ID signing and notarization for the first release.
- Schema design, migrations, and dedicated DDL editors; custom SQL remains available for DDL.
- Reproducing TablePlus or lazygit feature-for-feature.
