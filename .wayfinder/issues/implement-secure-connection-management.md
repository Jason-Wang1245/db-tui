---
title: Implement secure connection management
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - bootstrap-the-project-and-ci.md
  - define-connection-profile-behavior.md
---

## Question

Implement and verify the connection manager, host/port and URL profiles, validation and connection testing, local profile persistence, macOS/Linux keychain integration, password lifecycle, deletion, and actionable connection errors according to the resolved behavior.

## Resolution

Completed secure connection management on 2026-08-13.

### Connection launcher

- Replaced the bootstrap screen with a responsive profile list and create/edit form, including keyboard-complete and mouse-assisted selection, visible focus, long-list scrolling, minimum-size protection, and safe-default delete confirmation.
- Implemented field profiles, creation-only masked URL import, canonical validation, unique names, SSL modes, advanced parameters, explicit empty passwords, stored-secret markers, and preservation of failed/cancelled drafts.
- Added distinct **Save**, cancellable 10-second **Test**, and **Connect** flows. Tests do not persist or update last-used; Connect saves and updates last-used only after a successful ping, then hands the live session to the root workspace surface.
- Added monotonically identified typed intents/results, root-owned asynchronous commands and cancellation, stale-result rejection, stale-session closure, and rerunnable cancelled operations.

### Persistence and secrets

- Added a versioned JSON repository below `os.UserConfigDir` with `0700` directory and `0600` file modes, symlink rejection, size/version/schema checks, future-field preservation, and synced atomic temp-file replacement.
- Saved only canonical non-secret fields, stable RFC 4122 UUIDs, preferences, and timestamps. Password/URL/secret fields are rejected from persisted documents.
- Added coordinated system-keychain storage through `go-keyring`, keyed only by profile UUID, plus process-local session fallback when password saving is off or the keychain cannot accept a new secret.
- Turning password saving off removes the keychain entry; deleting a profile removes both persisted metadata and keychain/session secrets. Repository failures roll back keychain mutations when possible.

### PostgreSQL and safety

- Added the pgx connector for bounded pooled sessions, connect-and-ping testing, server/database/version/latency reporting, and injected clock use.
- Classified cancellation, timeout, authentication, missing database, permission, TLS, DNS, refusal, and general network failures into actionable safe summaries.
- Preserved sanitized PostgreSQL SQLSTATE/detail/hint metadata, while bounding server text and rejecting terminal-control injection, oversized input, multiple hosts, core/secret parameter overrides, and malformed URLs.
- Documented launcher controls, config permissions, macOS Keychain behavior, Linux Secret Service requirements, and session-only fallback. Updated the pinned toolchain to Go 1.26.6 after the current vulnerability database identified standard-library fixes in that patch.

### Verification

- `gofmt`, `git diff --check`, `go vet ./...`, `go test ./...`, and `go test -race ./...` pass.
- PostgreSQL Testcontainers integration tests pass against majors 14 and 18, including real connect/ping/version reporting and authentication-error classification.
- `govulncheck ./...` reports no reachable known vulnerabilities with Go 1.26.6.
- Native tests exercise the keyring adapter with its in-memory provider; cross-platform builds exercise the macOS and Linux keyring implementations. Real native keychain/Secret Service acceptance remains part of the already-defined release hardening matrix.

No new ticket is required. This closes the connection-management blocker for paginated browsing and the SQL workspace; connected-workspace behavior remains in [Implement the connected workspace shell](implement-the-connected-workspace-shell.md).
