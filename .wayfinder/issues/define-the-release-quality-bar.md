---
title: Define the release quality bar
label: wayfinder:grilling
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by: []
---

## Question

Which supported OS and architecture matrix, PostgreSQL compatibility range, automated and manual tests, accessibility expectations, documentation, artifact checks, and GitHub release criteria must be satisfied before the first release is complete?

## Resolution

Accepted the following first-release quality bar on 2026-08-13.

### Supported platforms and PostgreSQL

- Publish four CGO-free artifacts: macOS `amd64`, macOS `arm64`, Linux `amd64`, and Linux `arm64`.
- Support macOS 13 or newer and Ubuntu 22.04 or newer. Other current 64-bit Linux distributions are best-effort; saved passwords require an available Secret Service implementation, with session-only credentials as the supported fallback.
- Support PostgreSQL 14 through the newest generally available major at release time. A new major enters the supported range only after its compatibility matrix passes.
- Every release candidate must pass integration tests against every PostgreSQL major in that range. PostgreSQL 14 remains a v1 compatibility promise after its upstream support ends; current minor releases are recommended and older minors are best-effort.

### Required automated gates

- Every merge to protected `main` requires a clean `gofmt` check, `go vet ./...`, `go test ./...`, and `go test -race ./...`.
- Tests must cover reducer/state-machine behavior, rendering, persistence, SQL construction, critical failure paths, and the maintained regression corpus for designated fuzz targets.
- Normal CI runs integration tests against PostgreSQL 14 and the newest supported major; a release candidate runs the full supported-major matrix.
- `govulncheck ./...` must report no reachable known vulnerability.
- A GoReleaser snapshot must build all four artifacts. Every binary must pass `--version` and non-interactive startup smoke checks.
- Required-test flakiness is a failure; rerunning a job until it happens to pass is not an acceptable release path.
- Do not impose an arbitrary global coverage percentage. Review the coverage report and require explicit tests for every critical state transition, failure path, SQL builder, secret boundary, and persistence path.

### Native and manual acceptance

- Run automated smoke tests natively on all four OS/architecture targets; cross-compilation alone is insufficient.
- Run the complete hands-on acceptance suite from packaged artifacts on at least macOS `arm64` and Ubuntu `amd64`.
- Exercise each OS's default terminal, one documented modern terminal, and `tmux`; complete both keyboard-only and mouse-assisted passes.
- Cover fresh start, real keychain and session-only credentials, connect/reconnect, object browsing, pagination, staged CRUD and conflicts, SQL batches, bounded results, cancellation and errors, dirty-close protections, layout breakpoints/resizing, and clean quit.
- Acceptance permits no crash, hang, credential exposure, silent data loss, uncertain transaction outcome, or unresolved release-blocking defect.

### Accessibility and terminal behavior

- Every action must be available without a mouse, and focus and interaction mode must always be visibly identifiable.
- Color may reinforce but never solely communicate selection, dirty state, running work, warnings, or errors; text or symbols must carry the same meaning.
- Provide built-in high-contrast and no-color modes, and show terminal-compatible fallback keys in contextual help.
- Test Unicode display width, keyboard traversal, focus restoration, and layout at every established terminal-size breakpoint.
- Document that terminal UI constraints prevent a guarantee of full screen-reader compatibility; do not claim WCAG conformance.

### Documentation

- The README must cover installation, quick start, screenshots, supported platforms and PostgreSQL versions, project scope, and known limitations.
- Document profile setup, macOS keychain behavior, Linux Secret Service requirements, and the session-only fallback.
- Include a complete keyboard/mouse reference and guides for browsing, staged CRUD, SQL execution, cancellation, reconnect, and recovery.
- State the safety model explicitly: atomic staged applies, optimistic conflict handling, dirty-tab protections, and direct SQL execution without client-side destructive-statement classification.
- Provide troubleshooting, upgrade and uninstall instructions, a changelog, and a private security-reporting process. `--help` and `--version` must work without entering the TUI.
- This primarily personal-use project requires no project license or license file. If its repository is publicly readable, the absence of a license intentionally grants no permission for third-party reuse.

### Performance and resilience

- On representative supported hardware, local startup must reach the connection launcher within one second.
- Navigation and editing must remain responsive, with no database, keychain, filesystem, or other blocking I/O on the UI update path.
- Work lasting longer than 250 ms must show progress or a spinner within that interval and remain cancellable where its contract permits cancellation. Database and network latency is reported clearly but excluded from local UI timing targets.
- Acceptance data includes at least 1,000 visible relations, tables with 100 columns, and maximum-sized captured SQL results.
- A four-hour mixed browsing/editing/query/cancellation soak must show no crashes, deadlocks, runaway goroutines, or material unbounded memory growth.

### Security and privacy

- Passwords and complete connection URLs must never appear in config files, logs, user-facing errors, panic output, or committed test fixtures.
- Automated tests cover secret redaction, keychain failure, restrictive config permissions, connection-URL import, and profile/secret deletion.
- The application has no telemetry, analytics, automatic crash reporting, or unexpected network communication.
- Dependencies and their licenses receive review; Go modules and build inputs are pinned for reproducible resolution.
- Any known credential leak, unsafe identifier/value construction, or reachable high/critical vulnerability blocks release.

### Artifacts and GitHub publication

- Produce four versioned archives containing the correct binary, README, and embedded version/commit metadata. A license file is not required.
- Publish SHA-256 checksums, a per-artifact SBOM, and GitHub build-provenance attestations. Verify archive contents, clean extraction, hashes, metadata, SBOMs, and attestations before publication.
- macOS binaries are intentionally unsigned and unnotarized; Apple Developer ID membership and credentials are not required. Installation docs and release notes must clearly explain the resulting Gatekeeper behavior and how to verify the downloaded artifact safely.
- Create a semantic-version tag only from clean, protected `main`, after every automated, compatibility, native-smoke, manual, documentation, security, performance, and artifact gate passes.
- Prepare a draft GitHub release, download its actual artifacts, and rerun the release smoke checks before publishing it.
- Release notes list features, compatibility, limitations, checksums, and upgrade instructions. Published artifacts are immutable; a fix uses a new version rather than replacing files, and the release process includes a documented rollback response.

### Release-blocking defects

- Block on any crash, hang, data loss or corruption, credential exposure, unsafe SQL construction, uncertain transaction outcome, broken core flow on a supported platform, keyboard-inaccessible action, unreadable focus/mode state, or serious layout failure at a supported size.
- A minor defect may ship only when it has a practical documented workaround and a tracked follow-up; otherwise it remains release-blocking.

No new ticket is required. CI implementation belongs to [Bootstrap the project and CI](bootstrap-the-project-and-ci.md); integrated acceptance and defect closure belong to [Harden navigation, reliability, and polish](harden-navigation-reliability-and-polish.md); artifact verification and publication belong to [Package and publish the first release](package-and-publish-the-first-release.md). Packaged-binary findings remain in the map's existing fog until native testing makes them concrete.
