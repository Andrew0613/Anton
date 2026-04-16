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
- Treat `codex-threads` as a dependency or adapter surface, not as something to overwrite.
- Prefer a thin repo-local adapter model over hardcoded support for one project.
- Keep `AGENTS.md` and `README.md` short; put detailed design in `docs/`.

## Current Mission

Anton is still in the planning stage.

The next implementation agent should:

1. bootstrap the CLI skeleton
2. implement `doctor`, `task-state`, and `threads` as the first three usable surfaces
3. keep the command surface small and stable

## Doc Map

- Research memo: `docs/research/2026-04-16-anton-research-memo.md`
- Requirements: `docs/plans/2026-04-16-anton-requirements.md`
- Implementation plan: `docs/plans/2026-04-16-anton-implementation-plan.md`
- Handoff: `docs/handoffs/2026-04-16-anton-handoff.md`
