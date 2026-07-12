---
name: staging-state-machine
description: >-
  Load when changing the staging reducers or executor
  (internal/staging/transition/), or when reasoning about add/edit/delete/tag
  transitions, auto-skip/auto-unstage, the tag cascade, or conflict detection.
  Points to the authoritative state-machine reference.
---

# Staging state machine

The full, authoritative reference is **`docs/staging-state-transitions.md`**.
Read it before touching `internal/staging/transition/`. This skill is an
orientation only — it does not restate the tables or diagrams.

The staging system is provider-neutral (AWS, Google Cloud, Azure) and uses a
Redux-like pattern: pure reducers compute the next state, an executor persists
it.

## Which section answers which question

| Question | Section of the doc |
|----------|--------------------|
| What entry states exist and how add/edit/delete/reset move between them | Entry State Machine (states, diagram, transition rules) |
| When an edit is silently dropped or reverts a staged change | Special Behaviors → Auto-Skip / Auto-Unstage |
| Why deleting a `Create`-staged resource also drops its tag changes | Special Behaviors → Tag Cascade on Create Delete |
| What existence checks gate add/delete/tag | Special Behaviors → Resource Existence Checks |
| How tag additions/removals are tracked and reconciled | Tag State Machine (states, tracking, transition rules, delete-staged restriction) |
| How out-of-band writes are detected at apply time | Conflict Detection (including the Azure Key Vault second-granular timestamp note) |

## Implementation map

The state machine lives in `internal/staging/transition/`:

- `state.go` — state type definitions
- `action.go` — action type definitions
- `reducer.go` — pure reducer functions (`ReduceEntry`, `ReduceTag`)
- `executor.go` — persists reducer results to the store

Reducers are pure and deterministic; keep new behavior in the reducer and let
the executor stay a thin persistence step. When you change a transition, update
`docs/staging-state-transitions.md` to match.
