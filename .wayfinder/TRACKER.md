# Local Markdown Issue Tracker

This repository uses `.wayfinder/issues/*.md` as its issue tracker until an external tracker is configured.

## Issue format

- YAML frontmatter stores tracker state and relationships.
- `title` is the human-facing identity; use it instead of a bare file name or ID.
- `parent` points to the map file name.
- `blocked_by` is the local fallback for a native blocking relationship.
- An open ticket is claimed by setting `assignee` before any work begins.
- A resolution is recorded under `## Resolution`; then `status` is changed to `closed`.

## Wayfinding operations

- Find maps: `rg -l '^label: wayfinder:map$' .wayfinder/issues`
- Find a map's children: `rg -l '^parent: database-tui-from-concept-to-release.md$' .wayfinder/issues`
- Find open tickets: `rg -l '^status: open$' .wayfinder/issues`
- Find unclaimed tickets: inspect open children whose `assignee` value is empty.
- Find the frontier: among open, unclaimed children, select tickets whose `blocked_by` list is empty or whose blockers are all closed.
- Claim a ticket: set its `assignee` before doing any ticket work.
- Wire dependencies: create all issue files first, then add their file names to `blocked_by` in a second pass.
- Resolve a ticket: append the answer under `## Resolution`, set `status: closed`, and add a one-line linked gist to the map's `## Decisions so far`.
- Add context: link committed artifacts or branches from the ticket's resolution rather than pasting large assets into the map.

