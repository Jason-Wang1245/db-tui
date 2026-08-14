---
title: Bootstrap the project and CI
label: wayfinder:task
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - choose-the-application-architecture.md
  - define-the-release-quality-bar.md
---

## Question

Implement and verify the chosen Go module structure, application entry point, dependency baseline, formatting and linting, unit-test harness, integration-test infrastructure, and continuous-integration checks needed by every later vertical slice.

## Resolution

Completed the project and CI bootstrap on 2026-08-13.

### Module and executable

- Initialized `github.com/Jason-Wang1245/db-tui` for Go 1.26 with the patched `toolchain go1.26.6` and committed `go.mod`/`go.sum` dependency locks.
- Pinned Bubble Tea v2, Bubbles v2, Lip Gloss v2, pgx v5, OS keyring, Testcontainers PostgreSQL, and the `govulncheck` Go tool to exact releases.
- Added the single `cmd/db-tui` composition root with non-interactive `--help` and `--version`, build-time version/commit/date injection, error exit codes, and a functional Bubble Tea v2 bootstrap launcher.
- Added a minimal README, development targets, and repository ignores; no license file was added per the release-quality decision.

### Architectural seams

- Established `app`, `launcher`, `workspace`, `grid`, `sqltab`, `profile`, `postgres`, `platform`, `core`, and focused `ui` packages with the chosen dependency direction.
- Added opaque operation/request identities, safe structured errors, a bounded in-memory diagnostic log, workspace-scoped cancellation registry, tab/catalog/grid/SQL service contracts, profile repository/secret abstractions, and injected platform paths/clock/keyring adapters.
- Added a pgxpool-backed session that performs a real ping before becoming usable and returns sanitized domain errors.
- Added an architecture test that rejects concrete adapters in `app`, cross-feature production imports, UI dependencies in adapters, and internal dependencies from `core`.

### Tests and automation

- Added deterministic unit tests for CLI parsing, root model behavior, request identity, error redaction, cancellation, diagnostics, geometry, session secrets, config paths, and pre-cancelled keyring calls.
- Added a build-tagged Testcontainers PostgreSQL integration suite with configurable server-major selection.
- Added formatting, vet, unit/coverage, race, vulnerability, integration, build, and release-snapshot Make targets plus reusable formatting, native-smoke, and archive-verification scripts.
- Added normal GitHub CI with PostgreSQL 14/latest integration jobs and a GoReleaser snapshot gate. Added a manually triggered release-candidate workflow covering PostgreSQL 14–18 and native smoke builds on Linux/macOS `amd64`/`arm64` runners.
- Added GoReleaser v2 configuration for the four CGO-free archives, embedded metadata, README inclusion, checksums, and per-archive SBOMs; added weekly Go module and GitHub Actions dependency updates.

### Verification

- `go mod tidy -diff`, `gofmt`, `go vet ./...`, `go test ./...`, and `go test -race ./...` pass.
- `govulncheck ./...` reports no reachable vulnerabilities.
- The integration suite passes against real PostgreSQL 14 and PostgreSQL 18 Testcontainers instances.
- GoReleaser v2.17.1 validates the configuration and builds all four snapshot archives. Archive contents, SHA-256 checksums, embedded metadata, and native macOS `arm64` `--help`/`--version` smoke checks pass. Local snapshot validation skipped only SBOM generation because Syft is supplied by CI; CI requires and verifies all four SBOMs.

No new ticket is required. The bootstrap unblocks [Implement secure connection management](implement-secure-connection-management.md) and [Implement the connected workspace shell](implement-the-connected-workspace-shell.md).
