---
name: bug-audit-epic
description: Use when running a codebase-wide or subsystem bug audit — fan out orthogonal parallel investigators, adversarially verify every Medium+ finding before filing, write bug issues in the standard body, and file a severity-grouped [Epic] with linked sub-issues and an "Areas verified clean" section.
---

# Bug audit epic

A bug audit produces **verified bug reports only — no code changes**. The fixes come later, one PR per issue (see the `bug-fix-pr` skill). Evidence for this procedure: epics #493 (24 children), #543 (47 children, rounds 2–3), #570 (staging deep-dive); standard child body #545; children #545–569.

## Procedure

### 1. Fix the audit baseline

Pick the commit the audit runs against and record it in the epic verbatim: `Audited at main = <sha>` (#570 audited at `d424c74`; #493 at its round-1 base). Every `file:line` cite in every child must resolve against that SHA. When the audit spans several days and later rounds re-verify cites against a newer SHA, state both bases (#543 records base `98f5b74`, extended to `d424c74`).

### 2. Fan out orthogonal investigators

Run parallel investigators, each with a distinct attack angle so coverage does not overlap:

- **Subsystem audit** (#570, six angles): exhaustive transition-matrix analysis (actions × entry states × tag presence × remote state) with use-case-level reproduction; cross-check every parallel implementation of the same operation for drift (the three apply paths — global Runner / ApplyUseCase / GUI); the persistence/serialization layer; export/import data semantics; the GUI Go layer and its frontend contract; the Svelte view read as a state machine.
- **Whole-codebase audit** (#493, four rounds): ~11 area investigators (CLI, staging engine, export/import, crypto/key handling, each provider adapter, GUI, concurrency, cross-cutting), then an independent adversarial refutation round, then a completeness-critic pass with gap follow-ups, then a final verification round that declares the audit converged.

### 3. Adversarially verify every Medium+ candidate

Before a Medium-or-higher candidate is filed, an independent agent tries to refute it. Record the outcome in the epic methodology: upgrades (evidence traced deeper — #570 upgraded one to SDK-serializer level), confirmations, and downgrades with corrections (#543 downgraded five to Low). Drop plausible-but-unproven low-severity candidates rather than filing them (#493).

### 4. Deduplicate and exclude prior waves

Merge findings that independent investigators reported for the same root cause. Exclude anything already filed in earlier waves, stated explicitly by issue-number range (#570 excludes everything in #493 and #543 including staging items #521–#542; #543 excludes #493's #447 as tracked). Refute suspected process gaps in writing rather than leaving them implied (#570 refuted a suspected export tag-destruction; #543 dismissed a "closed but unmerged" concern by confirming the PR was merged).

### 5. File each child with the standard body

Body sections, in order (template = #545):

- `## Summary` — what breaks and why, in prose.
- `## Where` — `file:line` anchors for every implicated site.
- `## Failure scenario` — a concrete reproduction a fixer can replay.
- `## Severity` — with justification.
- `## Suggested fix` — the minimal correct change.
- Provenance/confidence footer — e.g. `Found during a Fable-coordinated / Opus-executed <scope> audit; adversarially verified (<how>). Confidence: CONFIRMED.`

Labels: `bug` + the area label(s) (`staging`, `param`, `secret`, …) + a `Priority:*` label. Priority definitions: `Critical` = silent data loss or wrong values returned (e.g. #545 — a tag-only write silently corrupts a Key Vault reference into a plain literal).

### 6. File the epic

- Title `[Epic] <scope> bug audit (<date>)`, label `epic` (+ area label for a subsystem audit).
- A **methodology** paragraph naming the investigators, the adversarial round and its upgrade/downgrade tally, and the convergence criterion.
- An **"Areas verified clean"** (or "Areas audited clean") paragraph listing what was checked and found sound. This is load-bearing: it stops the next wave from re-auditing the same ground.
- Children grouped by severity (Critical / High / Medium / Low) as checkbox lists, and also linked as GitHub sub-issues.
