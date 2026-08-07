---
title: Define connection profile behavior
label: wayfinder:grilling
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by: []
---

## Question

What fields, validation, test-connection flow, create/edit/delete UX, password/keychain lifecycle, connection-URL precedence, reconnect behavior, and safe config format should the macOS/Linux connection manager use?

## Resolution

Accepted the following connection-profile contract on 2026-08-07:

### Fields and URL import

- Every saved profile is field-based. Its core fields are a unique profile name, host, port, database, user, password input, per-profile **Save password** preference, SSL mode, and advanced connection parameters.
- Port defaults to `5432`; SSL mode defaults to `prefer`.
- A connection URL is offered only while creating a profile as an import shortcut. Import parses the URL into the canonical fields; the original URL is never retained.
- An imported password is immediately separated from the URL and follows the normal keychain/session-only rules.
- `sslmode` becomes the normal SSL field. Other imported parameters are preserved in a collapsed, editable **Advanced parameters** section rather than silently discarded.

### Validation

- Profile name, host, database, and user are required. Profile names are unique, and ports must be in the range `1–65535`.
- Passwords are optional; connecting without one prompts for it.
- Advanced parameter names must be unique and cannot redefine core fields or contain a password.
- **Save** requires local validation but not network reachability.
- URL structures the field model cannot represent safely, including multiple hosts, fail clearly instead of being partially imported.

### Create, edit, test, connect, and delete

- **Test** validates the draft, then performs a cancellable connect-and-ping with a 10-second default timeout. It neither saves the profile nor changes its last-used timestamp.
- Test progress and elapsed time appear inline. Success reports the resolved server, database, PostgreSQL version, and latency. Failures classify authentication, DNS/network, TLS, timeout, and database errors while sanitizing credentials.
- **Save** persists without connecting. **Connect** validates and pings, saves only after success, updates last-used state, and enters the workspace. Failure preserves every draft value and shows an actionable inline error.
- Editing uses a draft; **Cancel** discards it. A stored secret is shown as **Stored in keychain**, never as fake masked text. **Replace password** is explicit; leaving the secret untouched preserves it.
- Deleting requires a safe-default confirmation naming the profile and explaining that its keychain secret will also be removed. An active workspace must disconnect before its profile can be deleted.

### Password lifecycle

- **Save password in system keychain** defaults on but is independently configurable for every profile.
- Passwords are stored under the profile's stable UUID, never its display name or connection fields, and never in profile JSON.
- Turning password saving off and saving an existing profile removes its keychain entry. A newly entered password remains usable only for the current process and is requested again after restart.
- If the keychain is unavailable, the non-secret profile may still be saved, but the app clearly switches that credential to session-only behavior; plaintext persistence is never a fallback.
- Deleting a profile removes its keychain entry.

### Reconnection

- Connection loss preserves tabs, SQL text, staged grid changes, and view state in memory, while displaying a persistent **Disconnected — Reconnect** status.
- Reconnect uses the profile plus any available keychain or session password.
- Queries and mutations are never replayed automatically. After reconnecting, users explicitly refresh grids or rerun SQL.

### Persistence

- Store profiles in one versioned JSON file under the OS user-config directory, with a `0700` parent directory, `0600` file, and atomic temp-file-and-rename writes.
- Each record contains a stable UUID, canonical connection fields, advanced parameters, password-saving preference, and timestamps including last-used state. It contains neither passwords nor the original imported URL.
- Readers tolerate unknown future fields so compatible schema evolution does not destroy data.

No new ticket is required; implementation and verification belong to [Implement secure connection management](implement-secure-connection-management.md).
