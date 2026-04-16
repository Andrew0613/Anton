# Repository Agent Entry

This repo defines the reusable `Anton` harness.

## Startup

1. Read `README.md`.
2. Read `docs/research/2026-04-16-anton-research-memo.md`.
3. Read `docs/plans/2026-04-16-anton-requirements.md`.
4. Read `docs/plans/2026-04-16-anton-implementation-plan.md`.
5. Read `docs/handoffs/2026-04-16-anton-handoff.md` before starting implementation.

## Hard Rules

- Keep this repo focused on reusable harness infrastructure.
- Do not absorb repo-specific business logic from downstream projects.
- Treat `codex-threads` as a dependency surface, not as something to overwrite.
- Prefer one canonical Anton contract plus repo-local `anton.yaml` over repo-specific runtime adapters.
- Keep `AGENTS.md` and `README.md` short; put detailed design in `docs/`.

## Current Mission

Anton is in bootstrap implementation.

The next implementation agent should:

1. harden `doctor`, `task-state`, and `threads` around the canonical repo contract
2. keep repos adapting to Anton via `anton.yaml` and `.anton/tasks/...`
3. keep the command surface small and stable

## Doc Map

- Research memo: `docs/research/2026-04-16-anton-research-memo.md`
- Requirements: `docs/plans/2026-04-16-anton-requirements.md`
- Implementation plan: `docs/plans/2026-04-16-anton-implementation-plan.md`
- Handoff: `docs/handoffs/2026-04-16-anton-handoff.md`
