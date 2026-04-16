# Anton

Reusable harness CLI and thin skill layer for agentic repository workflows.

## Status

This repository is in the research-to-requirements phase.

The current goal is to define:

- what Anton should own
- what should remain in repo-local docs and scripts
- how Anton should integrate with `codex-threads`
- how a future implementation agent should bootstrap the first working version

## Why This Repo Exists

Recent research across `euresis`, local/remote `PhysEdit`, workspace-level agent entrypoints, and `codex-threads-insights` showed recurring problems:

- entrypoint drift across `AGENTS.md`, `CLAUDE.md`, README blocks, and workflow docs
- task-state conventions that are only partially enforced
- runtime harness behavior that diverges from documented repo workflow
- local vs remote environment differences that break otherwise-valid commands
- repeated context overload and weak closure signals in recent sessions

Anton exists to turn those recurring harness problems into a reusable product surface.

## Core Positioning

Anton is a new repo.

It does not replace `codex-threads`.

Current boundary:

- `Anton`
  - harness CLI
  - task-state lifecycle
  - entrypoint generation/checking
  - execution-context resolution
  - local/remote preflight
  - thin skill wrappers
- `codex-threads`
  - memory
  - trace
  - recent thread lookup
  - insight generation
  - evidence-linked receipts

## Initial Scope

Planned v0 modules:

- `entrypoint`
- `task-state`
- `context`
- `threads`
- `doctor`

Planned v0 commands:

- `anton doctor`
- `anton init`
- `anton task init`
- `anton task pulse`
- `anton task check`
- `anton context resolve`
- `anton entrypoint check`
- `anton entrypoint sync`
- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`

## Docs

- Research memo: [docs/research/2026-04-16-anton-research-memo.md](docs/research/2026-04-16-anton-research-memo.md)
- Requirements: [docs/plans/2026-04-16-anton-requirements.md](docs/plans/2026-04-16-anton-requirements.md)
- Implementation plan: [docs/plans/2026-04-16-anton-implementation-plan.md](docs/plans/2026-04-16-anton-implementation-plan.md)
- Handoff: [docs/handoffs/2026-04-16-anton-handoff.md](docs/handoffs/2026-04-16-anton-handoff.md)

## Development Notes

The first implementation pass should stay narrow:

- no orchestration daemon
- no PR/deploy automation
- no queueing/runtime scheduler
- no attempt to absorb the full `codex-threads` repo

The right first milestone is a stable CLI that can unify current harness pain across local repos and remote SSH environments.
