---
title: Define SQL tab execution behavior
label: wayfinder:grilling
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by: []
---

## Question

How should SQL tabs handle editing, statement selection, multi-statement input, execution and cancellation, transactions, result sets versus command results, errors, result pagination, dirty-close confirmation, and keyboard/mouse interaction without history or saved queries?

## Resolution

Accepted the following SQL-tab contract on 2026-08-10.

### Editing and execution target

- Each session-local SQL tab owns an independent multiline Unicode buffer with bracketed paste, undo/redo, keyboard text selection, click-to-place-cursor, and mouse-drag selection.
- `Tab` inserts two spaces while editing. `Esc` leaves editor text mode and returns control to pane navigation, preserving the root focus model.
- If text is explicitly selected, **Execute** submits only that selection. Otherwise it submits the complete buffer. A separate **Run all** action always submits the complete buffer when a selection exists.
- V1 does not infer a statement under the cursor and does not provide syntax highlighting, autocomplete, or SQL formatting. PostgreSQL remains the SQL parser.
- `Ctrl+Enter` executes when enhanced keys are available and `F5` is the universal fallback. Execute, Run all, Rerun, and Cancel also have visible clickable actions.
- The active profile/database and whether Execute targets the selection or full buffer remain prominent. Execution begins immediately from the explicit action; the client does not attempt unreliable destructive-SQL classification or add a warning based on SQL text.

### Runs, batches, and sessions

- A run uses an immutable snapshot of its submitted text while the editor remains editable. One run may be active per tab, different tabs may run concurrently, and Execute is disabled in a tab until its run completes or is cancelled.
- Submit the selected text or buffer unchanged as one PostgreSQL batch. The app never splits, reorders, silently retries, or silently wraps statements in an extra transaction; PostgreSQL determines batch parsing and where execution stops on error.
- Transactions cannot span executions. `BEGIN` through `COMMIT` or `ROLLBACK` must occur in the same submitted batch, and a connection is checked out only for that run.
- Execution sessions are isolated. `SET`, temporary tables, prepared statements, `LISTEN`, and session advisory locks last only within the submitted batch.
- Before releasing a connection, roll back any transaction left open and reset session state. If reset cannot be verified, discard the connection. An unclosed transaction produces a clear warning rather than appearing fully successful.
- Do not replay any query or mutation automatically after cancellation, connection loss, or reconnect.

### Ordered outputs and bounded results

- Preserve every statement output in execution order as a numbered result item. Row-returning statements, including DML with `RETURNING`, receive read-only grids; commands receive compact command-tag and affected-row summaries. Notices attach to the run in their observed order.
- Result grids use the table grid's keyboard/mouse navigation vocabulary, distinguish `NULL` from empty strings, and show truncated single-line cell previews. `Enter` opens a full-value inspector.
- Users can copy one cell, one row, or selected rows through the established clipboard behavior. Result editing and file export are outside v1.
- Capture at most 10,000 rows per row result and show 100 rows per client-side page. Enforce a 64 MiB total captured-data limit per execution.
- Mark capped output **Truncated** and direct the user to narrow the query or add `LIMIT`. Never rerun arbitrary SQL automatically to obtain another page. Drain or stop remaining output safely, then release the connection once the run finishes.
- Command results show the command tag, affected-row count when PostgreSQL provides one, notices, and elapsed time.

### Cancellation, reruns, errors, and result lifetime

- `Ctrl+C` while a tab is running, or its clickable Cancel action, requests cancellation and changes status to **Cancelling...** until PostgreSQL responds.
- Completed outputs remain visible after cancellation. An interrupted row result may remain only with a prominent **Incomplete — cancelled** marker. Cancellation never masquerades as a complete result.
- A dedicated **Rerun** action submits the previous run's exact immutable SQL snapshot. Normal Execute uses the editor's current selection or buffer.
- A tab retains only its most recent execution, which is not query history. Previous results remain visible as **Stale** while a new run is active and are replaced as the new run produces output.
- A server error becomes an ordered result item after any earlier outputs. Show message, SQLSTATE, detail, hint, context, and statement position when supplied; map positions against the submitted snapshot and never overwrite current editor text.
- Failed tabs retain the `!` marker until revisited or rerun. Late async responses whose request identity no longer matches the active run cannot overwrite newer state.
- If cancellation invalidates a connection, the pool replaces it. Cancelled SQL remains available for deliberate Execute or Rerun.

### Dirty state and tab lifecycle

- A SQL tab is dirty whenever its buffer contains any text, including text that executed successfully. Running or cancelling does not mark it clean; clearing the buffer does.
- A dirty idle tab closes only through **Discard SQL / Cancel**. A running dirty tab uses **Cancel run and discard SQL / Keep open**. Empty idle tabs close immediately.
- Disconnect and quit summarize all dirty and running tabs in one safe-default confirmation rather than opening sequential prompts.
- The editor remains visible during execution. Focus moves to the result pane only when a result or error arrives, and tab switching preserves each tab's editor and result state.

No new ticket is required. Implementation belongs to [Implement the SQL tab workspace](implement-the-sql-tab-workspace.md); integrated cancellation and transaction-outcome recovery remain in the map's existing fog until real execution paths make them concrete.
