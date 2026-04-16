# Anton Handoff

Date: 2026-04-16

## What Was Decided

- `Anton` is a new GitHub repository: `Andrew0613/Anton`
- local path: `/Users/puyuandong613/workspace/opensource/Anton`
- `Anton` should be a reusable harness CLI plus thin skill layer
- `Anton v0` uses Go
- `Anton v0` is centered on:
  - `doctor`
  - `task-state`
  - `threads`
- execution context is still part of Anton, but should first ship through `doctor --json`
- `codex-threads` remains separate and is not Anton’s product center
- repos should adapt to Anton through `anton.yaml`, not through repo-specific runtime adapters
- the canonical task bundle layout is `.anton/tasks/active/<id_slug>/`

## Why This Repo Exists

Current research across `euresis`, local/remote `PhysEdit`, workspace-level harness docs, and `codex-threads-insights` showed:

- duplicated and drifting entrypoint docs
- brittle task-state practices
- manual bootstrap overhead
- runtime/doc contract mismatches
- remote environment and filesystem-specific failures
- repeated `Context overload` and weak closure patterns in recent sessions

## What Exists In This Repo Now

- `README.md`
- `AGENTS.md`
- `anton.yaml`
- `task_plan.md`
- `findings.md`
- `progress.md`
- `docs/research/2026-04-16-anton-research-memo.md`
- `docs/plans/2026-04-16-anton-requirements.md`
- `docs/plans/2026-04-16-anton-implementation-plan.md`
- this handoff doc
- `cmd/anton`
- `internal/adapter`
- `internal/doctor`
- `internal/taskstate`
- `internal/threads`

## Current Implementation State

- the CLI skeleton exists and builds around `doctor`, `task-state`, and `threads`
- runtime config is loaded from repo-local `anton.yaml` with canonical defaults
- `anton task-state` uses the canonical `.anton/tasks/...` layout
- `anton threads` resolves project scope from flag/env/configured workspace roots/repo root
- `anton doctor` should remain the place that emits the execution and config receipt
- repo-specific runtime adapters are explicitly out of scope for the main execution path

## Recommended Next Implementation Step

Keep hardening the canonical contract instead of expanding the command surface.

The next step should focus on:

1. tightening `status.yaml` schema expectations and fixtures around the canonical `.anton/tasks` contract
2. keeping `threads` thin while making project inference more auditable
3. preserving small, explicit command contracts instead of growing auxiliary surfaces

## Suggested First Coding Tasks

1. Add or refine fixtures for explicit config, default config, and canonical task bundles.
2. Harden `status.yaml` validation around the canonical schema.
3. Keep `threads` thin and evidence-first.
4. Tighten JSON/human output contracts where ambiguity remains.

## Open Questions

- How much machine-specific metadata should `Anton task` persist by default?
- How strict should Anton be about requiring explicit `anton.yaml` instead of falling back to defaults?
- How minimal should `anton doctor` stay in v0 before it starts absorbing too many policy checks?
- How thin should the first `threads` surface remain before it starts duplicating `codex-threads` logic?

## Constraint To Preserve

Do not expand Anton into a large orchestration runtime in the first implementation wave.

Keep it CLI-first, canonical, config-driven, and tightly scoped to the recurring harness problems already observed.
