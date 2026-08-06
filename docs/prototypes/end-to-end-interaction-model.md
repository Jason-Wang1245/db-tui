# End-to-end interaction model — prototype v0.1

Status: under review for the Wayfinder ticket [Prototype the end-to-end interaction model](../../.wayfinder/issues/prototype-the-end-to-end-interaction-model.md).

This is a deliberately rough interaction contract. It chooses the shell, focus, and feedback model while leaving database semantics to the connection, browsing/CRUD, and SQL decision tickets.

## Design test

The model succeeds when a user can connect, open a table, stage an edit, inspect or discard it, run SQL, move among mixed tabs, and safely leave using the same small vocabulary:

- Arrows or `h/j/k/l` move within a focused pane.
- `Enter` opens, activates, or begins the highlighted action.
- `Esc` backs out one interaction level: edit → pane navigation → modal cancellation.
- `Tab` / `Shift+Tab` move between visible panes while in navigation mode.
- `[` / `]` select the previous or next workspace tab.
- `?` opens contextual help. Mouse actions mirror visible keyboard actions.

## Application hierarchy

```text
Application
├── Connection launcher
│   ├── Profile list
│   ├── Profile details / editor
│   └── Persistent status line
└── Connected workspace
    ├── Connection header
    ├── Object navigator
    ├── Mixed tab strip
    ├── Active tab
    │   ├── Table: data grid
    │   └── SQL: editor + result
    ├── Contextual key hints
    └── Persistent status line

Modal overlay (at most one; owned by the application root)
```

The connection launcher and connected workspace are separate top-level surfaces. The launcher is not squeezed into the workspace sidebar. The workspace always names the active profile/database in its header.

## Wide workspace

```text
┌ dbtui ─ local-dev / postgres ─────────────────────────────── connected ┐
│ OBJECTS               │ [users •] [orders] [SQL 1*] [+ SQL]            │
│ ▾ public              ├─────────────────────────────────────────────────┤
│   ▸ orders            │ id │ email             │ active │ created_at   │
│   ▾ users             │ 41 │ sam@example.com   │ true   │ 2026-08-02   │
│     columns           │ 42 │ jo@example.com  ✎ │ true   │ 2026-08-03   │
│ ▸ audit               │ 43 │ lee@example.com   │ false  │ 2026-08-04   │
│                       │                                         1–50/238│
├───────────────────────┴─────────────────────────────────────────────────┤
│ 1 staged change · Enter edit · i insert · d delete · A apply · R revert│
│ Ready                                                   ? help · q quit │
└─────────────────────────────────────────────────────────────────────────┘
```

- The active pane has one strong border/accent and its selected item has a second, subtler highlight.
- A tab marker `*` means unsaved SQL text; `•` means staged grid mutations. Running tabs use a spinner; failed tabs use `!` until revisited or rerun.
- The last two rows are stable: contextual actions above, application/job status below. Content never shifts when a message appears.

## Focus model

The root model owns a single focus path rather than letting components infer focus independently:

```text
launcher.profiles
launcher.details
workspace.navigator
workspace.tabs
workspace.table.grid
workspace.sql.editor
workspace.sql.result
modal.<kind>
```

- `Tab` and `Shift+Tab` cycle only through panes currently visible.
- Clicking a pane focuses it before applying the click.
- Opening a table focuses its grid. Opening a SQL tab focuses its editor.
- Switching tabs restores that tab's last internal focus, cursor, scroll, and selection.
- `Esc` in a cell or SQL editor returns to pane navigation. It does not unexpectedly switch panes.
- When text is being edited, printable keys, arrows, `Home`, `End`, and `Tab` belong to the editor; `Tab` inserts two spaces in SQL. `Esc` returns shortcut control to the shell.
- A modal saves the prior focus path, captures all input, and restores that path on close.

## Keyboard map

Bindings are layered so the same key has one meaning at a time.

| Scope | Key | Action |
| --- | --- | --- |
| Everywhere | `?` | Contextual help overlay |
| Everywhere | `Ctrl+C` | Cancel the active operation; if idle, request quit |
| Navigation mode | `Tab` / `Shift+Tab` | Next / previous visible pane |
| Navigation mode | `h/j/k/l`, arrows | Move in the focused pane |
| Navigation mode | `Enter` | Open / activate / edit selected item |
| Workspace | `[` / `]` | Previous / next tab |
| Workspace | `n` | New SQL tab when focus is on the tab strip |
| Workspace | `x` | Close selected tab when focus is on the tab strip |
| Navigator | `Space` | Expand / collapse schema or table metadata |
| Navigator | `r` | Refresh visible objects |
| Table grid | `e` or `Enter` | Edit focused cell |
| Table grid | `i` | Stage an inserted row |
| Table grid | `d` | Toggle staged deletion for the focused row |
| Table grid | `A` | Review and apply staged mutations |
| Table grid | `R` | Review and revert staged mutations |
| SQL editor | `Ctrl+Enter` | Execute, when enhanced keys are available |
| SQL editor | `F5` | Execute fallback in every supported terminal |
| SQL editor | `Ctrl+C` | Cancel the running execution |
| Modal | `Esc` | Choose the safe/cancel path |
| Modal | `Tab`, arrows | Move among choices |
| Modal | `Enter` | Activate the focused choice |

The footer exposes only actions relevant to the focused pane. Full help shows both the preferred binding and any terminal-compatible fallback.

## Mouse contract

- Single-click focuses a pane and selects the clicked tree item, tab, row, cell, button, or modal action.
- Double-clicking a grid cell begins editing; single-click plus `Enter` is equivalent.
- Clicking disclosure markers expands/collapses; clicking a table name opens or focuses its existing tab.
- The wheel scrolls the pane under the pointer without moving keyboard focus. Shift+wheel scrolls a grid horizontally where the terminal reports it.
- Clicking a tab activates it; its visible close target follows the same clean/dirty close rules as `x`.
- Right-click and hover are never required. Drag is not required for v1. Every mouse action has a visible keyboard equivalent.

## Tab lifecycle

- Table and SQL tabs share one ordered strip and may coexist in any order.
- Opening an already-open table focuses its existing tab rather than duplicating it.
- A new SQL tab receives the next session-local label (`SQL 1`, `SQL 2`, …) and is not persisted.
- Each tab owns its view state and async request identity. Background completions may update their owning tab but never steal focus.
- Closing a clean, idle tab is immediate.
- Closing a dirty SQL tab prompts **Discard / Cancel**; saving is absent because saved queries are out of scope.
- Closing a table tab with staged mutations prompts **Discard changes and close / Cancel**.
- Closing a running tab prompts **Cancel operation and close / Keep open**.
- Disconnecting or quitting summarizes all dirty/running tabs in one modal rather than opening a sequence of prompts.

## Core flows

### Connect

1. Launch into the profile list with the most recently used profile selected.
2. `Enter` connects; `n` or the visible New action opens the detail editor.
3. Test Connection reports progress and its result inline without leaving the form.
4. Successful Connect replaces the launcher with the workspace and focuses the object navigator.
5. Connection failure stays on the launcher, keeps entered values, and focuses an actionable error summary.

### Browse and stage

1. Expand a schema and open a table from the navigator.
2. A new table tab opens and the first data cell receives focus.
3. Editing, insertion, and deletion change only the tab-local staged set; decorations and the tab marker update immediately.
4. `A` opens an apply-review modal; `R` opens a revert-review modal. Their exact database semantics belong to the staged CRUD decision.
5. Apply/revert progress appears in the stable status line. Success clears the tab marker; failure persists inline and preserves recoverable staged input.

### Run SQL

1. Focus the tab strip and press `n`, or click **+ SQL**.
2. Type in the editor. `Ctrl+Enter` (or `F5`) starts execution and moves focus to the result only when a result or error arrives.
3. While running, the editor remains visible and status shows elapsed time plus `Ctrl+C cancel`.
4. A row result appears below the editor; a command result occupies a compact result pane; an error appears at the top of the result pane.
5. `Esc` from editor text mode restores pane navigation, after which `Tab` moves between editor and result.

## Modal rules

- One centered overlay, one dimmed background, no nested modals.
- Title states the consequence, body identifies the affected profile/tab/rows, and choices use verbs.
- Initial focus is always the non-destructive choice.
- `Esc` always takes the non-destructive path.
- Destructive actions require a second explicit `Enter` only through the modal, never a timed toast or hidden confirmation.
- Help is an overlay but has only Close; errors that need copied detail use an inline expandable panel instead of a modal.

## Status feedback

| Kind | Surface | Lifetime |
| --- | --- | --- |
| Selection/focus | Border, row/cell highlight | While focused |
| Dirty state | Tab marker + cell/row decoration + contextual count | Until applied/reverted/discarded |
| Running work | Spinner in owning tab + status text + cancel hint | Until completion |
| Success | Status line | Until the next meaningful action |
| Recoverable error | Owning pane plus `!` tab marker | Until dismissed or superseded |
| Connection loss | Persistent banner/status with reconnect action | Until recovered or disconnected |
| Confirmation | Modal | Until explicit choice |

Late async responses are ignored when their request identity no longer matches the tab's active request.

## Terminal degradation

Breakpoints are behavioral starting points to validate during implementation, not immutable pixel-perfect constants.

| Available size | Layout |
| --- | --- |
| At least 100×24 | Navigator and active tab side by side; SQL editor/result split vertically |
| 70–99 columns or 16–23 rows | Navigator becomes an on-demand drawer; active tab gets full width; SQL shows editor or result, toggled by focus cycling |
| 48–69 columns or 12–15 rows | Single-pane mode; condensed header; horizontally scrollable tab strip; footer shows one primary hint plus `? more` |
| Below 48×12 | Non-destructive minimum-size screen showing required/current size, connection/job state, and quit; all tab state remains in memory |

- Column content truncates by display width; data never wraps inside grid rows.
- Horizontal grid navigation scrolls whole columns into view and keeps row identity columns pinned when feasible.
- Modals clamp to the terminal and become full-surface prompts in single-pane mode.
- Resizing preserves focus by identity. If its pane becomes hidden, focus moves to that pane's compact representation and returns when expanded.

## Decisions intentionally deferred

- Profile fields, URL precedence, secret lifecycle, reconnection policy, and config format.
- Pagination/order, editable-table eligibility, conflict detection, value validation, and transaction semantics.
- Statement selection, multi-statement behavior, transaction handling, and exact SQL result semantics.
- Final accessibility and supported-terminal acceptance criteria.

Those decisions may refine labels or states, but should preserve this root-owned focus model, layered shortcuts, mixed-tab shell, persistent status surface, safe-modal behavior, and responsive pane collapse unless prototype feedback rejects them.
