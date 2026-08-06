---
title: Prototype the end-to-end interaction model
label: wayfinder:prototype
status: closed
parent: database-tui-from-concept-to-release.md
assignee: codex
blocked_by: []
---

## Question

What concrete pane hierarchy, focus model, keyboard map, mouse behavior, tab lifecycle, modal flow, status feedback, and small-terminal degradation make connection management, browsing, staged editing, and SQL execution feel cohesive and lazygit-grade?

## Prototype

- [End-to-end interaction model — prototype v0.1](../../docs/prototypes/end-to-end-interaction-model.md)

## Resolution

Accepted the prototype on 2026-08-06. The interaction contract uses a root-owned single focus path, mode-aware layered shortcuts with terminal-safe fallbacks, full keyboard/mouse parity, mixed table and SQL tabs that preserve local state, safe root-level modals, stable hint/status rows, and progressive degradation from split panes to a drawer, single-pane layout, and minimum-size guard.

The linked prototype is the detailed decision record. Its intentionally deferred database semantics already belong to the existing connection-profile, record-browsing/staged-CRUD, and SQL-execution decision tickets, so this resolution surfaces no new ticket.
