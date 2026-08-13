---
title: Choose the application architecture
label: wayfinder:grilling
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by:
  - prototype-the-end-to-end-interaction-model.md
  - research-the-go-postgresql-tui-foundations.md
  - define-record-browsing-and-staged-crud-semantics.md
  - define-sql-tab-execution-behavior.md
  - define-connection-profile-behavior.md
---

## Question

What package boundaries, Bubble Tea model/message/command ownership, database service interfaces, tab state model, concurrency and cancellation strategy, error model, configuration abstractions, and dependency rules will keep the application testable and responsive as the decided workflows are implemented?

## Resolution

Accepted the following application architecture on 2026-08-13.

### Modular-monolith package structure

- Ship one Go binary. `cmd/db-tui` owns flags, `--help`/`--version`, dependency construction, and Bubble Tea startup.
- Organize implementation under feature-oriented internal packages: `app` for the root model and lifecycle, `launcher` for profiles and connection flow, `workspace` for catalog navigation and the mixed-tab shell, `grid` for table browsing and staged mutations, `sqltab` for SQL editing/execution/results, `profile` for validation and persistence coordination, `postgres` for pgx-backed adapters, and `platform` for filesystem, keychain, clipboard, and clock adapters.
- Keep only opaque IDs, request metadata, and structured errors in a very small `core` package. Do not create a broad shared-domain or generic utility package.
- Use no plugin system, event bus, dependency-injection framework, separate worker process, or other distributed boundary in v1.

### Bubble Tea ownership and messages

- The root `app.Model` exclusively owns the active top-level surface, connection/workspace lifecycle, terminal size, responsive layout, focus path, ordered mixed tabs, active tab, root modals, and shutdown.
- Route input to exactly one focused child model. Feature models own only their local state and return typed intents; they never mutate sibling state or invoke services directly from `Update`.
- A user intent synchronously updates model state and returns a `tea.Cmd`. Each asynchronous request captures immutable input, workspace/tab/operation identity, a monotonically increasing request ID, and a cancellable context.
- Commands return typed progress or completion messages containing domain data or a structured error—never callbacks or direct UI mutations. The owning model accepts a message only when every identity and request ID still matches, so late work cannot overwrite newer state.
- Do not use global mutable state, a global event bus, or goroutines that mutate models outside the Bubble Tea message loop.

### Database capabilities

- One connected workspace owns one bounded `pgxpool` through a `Session` abstraction.
- Split PostgreSQL access into narrow capabilities: `Connector` for test/connect/ping/reconnect/close, `CatalogReader` for metadata and privileges, `TableBrowser` for deterministic pages and conflict reads, `MutationApplier` for one atomic staged batch, and `SQLExecutor` for one isolated SQL snapshot and its ordered outputs.
- Feature packages depend only on the capabilities they consume. Define interfaces beside the consuming feature; `postgres` implements them without importing Bubble Tea or presentation code.
- Keep `pgx`, PostgreSQL catalog details, query construction, and driver errors inside `postgres`. Only feature-owned domain DTOs and sanitized structured errors cross the adapter boundary.

### Tab and feature state

- Represent each tab with a shared envelope containing its stable ID, title, kind, lifecycle status, last focus, and active-request metadata, plus a kind-specific state value rather than one oversized cross-kind struct.
- `TableTabState` owns relation metadata, sort/page cursor, loaded rows, selection, staged changes, conflict data, and local errors.
- `SQLTabState` owns editor state, submitted snapshot, ordered outputs, client-side result pagination, run status, and local errors.
- The root owns tab order and the active tab ID; `grid` and `sqltab` own their respective reducers and rendering.
- Serializable/model state contains domain data only. It never contains a live pool, connection, service, context, channel, mutex, cancel function, or callback.

### Structured concurrency and cancellation

- Database, keychain, filesystem, and other potentially blocking operations run only in `tea.Cmd`; `Update` never blocks.
- Allow one active load or apply operation per table tab and one active execution per SQL tab. Independent tabs may work concurrently within the workspace pool's configured limit.
- Each workspace owns a cancellation scope. Disconnecting or closing it cancels all child operations.
- A small runtime cancellation registry maps opaque operation IDs to cancel functions. Models store only operation IDs and visible statuses; tab close, explicit cancellation, workspace disconnect, and shutdown signal the registry.
- Cancellation does not replace request-identity checks: stale completions are still rejected. A command owns and finishes every goroutine it starts; no detached worker survives its operation.

### Errors and recovery information

- Errors crossing service boundaries carry an operation, category, sanitized safe summary, retryability, and wrapped cause.
- Categories include validation, authentication, network, TLS, timeout, cancellation, permission, conflict, constraint, unsupported, persistence, keychain, and internal failures.
- PostgreSQL errors may additionally carry SQLSTATE, detail, hint, constraint name, statement position, and sanitized server context.
- Adapters remove credentials and connection URLs before an error leaves their package. Feature models choose UI placement and recovery actions; lower-level packages never render text or open modals.
- Preserve wrapped causes for tests and explicit diagnostics while showing only safe summaries by default.

### Profiles, configuration, and secrets

- `ProfileRepository` loads and atomically writes the complete versioned non-secret profile document. `SecretStore` gets, sets, and deletes passwords by stable profile UUID. `SessionSecrets` stores unsaved passwords in memory only, and `ConfigPaths` resolves platform paths and required permissions.
- A profile service coordinates repository and secret-store mutations, cleanup, and partial-failure reporting. Only profile/application orchestration may combine connection fields with a password.
- Concrete JSON-filesystem and operating-system-keychain implementations live in `platform` behind these interfaces.

### Dependency and rendering rules

- `cmd/db-tui` is the composition root and may import features plus concrete adapters for construction.
- `app` composes feature models and consumes interfaces, but does not import concrete PostgreSQL, filesystem, or keychain adapters.
- Features may import `core` and focused UI primitives, but not one another's internals. Each feature owns its domain types; orchestration performs explicit conversions instead of importing another feature merely to reuse a struct.
- `postgres` and `platform` contain no Bubble Tea or UI dependencies. Packages below `app` do not directly read globals or environment variables, exit the process, inspect terminal state, or read wall-clock time.
- Use constructor injection and small handwritten fakes. Enforce forbidden imports with architecture tests or static checks.
- Rendering is pure: models produce view data, the root computes responsive rectangles and explicit hitboxes, and feature renderers receive their rectangle, focus/mode, and theme. Renderers neither mutate state nor start commands.
- Keyboard and mouse inputs resolve to the same typed intents. Keep raw/typed PostgreSQL values separate from display strings.

### Testing seams and diagnostics

- Reducer tests cover every meaningful transition, including stale messages and cancellation races. Snapshot/golden and semantic tests cover focus, available actions, themes, Unicode width, and terminal breakpoints.
- Share contract suites between real and fake profile, keychain, catalog, browsing, mutation, and SQL adapters. Use real PostgreSQL integration tests for transactions, types, privileges, conflicts, cancellation, and recovery.
- Reserve full-program `teatest` coverage for high-value workflows; most state coverage belongs in deterministic lower-level tests.
- Inject clocks, ID generation, config paths/filesystems, clipboard behavior, and service fakes.
- Maintain only a bounded in-memory diagnostic log of sanitized lifecycle and operation events, exposed through a troubleshooting view or explicit dump action. Do not create a default log file, add telemetry, or log credentials, SQL text, parameter values, or returned data.

No new ticket is required. Establishing these packages, contracts, dependency checks, test seams, and construction belongs to [Bootstrap the project and CI](bootstrap-the-project-and-ci.md); feature-specific implementations remain in their existing task tickets.
