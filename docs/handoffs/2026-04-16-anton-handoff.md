# Anton Handoff

Date: 2026-04-16

## What Was Decided

- `Anton` is a new GitHub repository: `Andrew0613/Anton`
- local path: `/Users/puyuandong613/workspace/opensource/Anton`
- `Anton` should be a reusable harness CLI plus thin skill layer
- `codex-threads` remains separate and should be treated as a backend adapter

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
- `task_plan.md`
- `findings.md`
- `progress.md`
- `docs/research/2026-04-16-anton-research-memo.md`
- `docs/plans/2026-04-16-anton-requirements.md`
- `docs/plans/2026-04-16-anton-implementation-plan.md`
- this handoff doc

## Recommended Next Implementation Step

Start with the CLI skeleton and implement only:

1. `anton doctor`
2. `anton context resolve`
3. `anton task init`

These three commands are the smallest slice that attacks real current pain.

## Suggested First Coding Tasks

1. Choose the implementation language and package layout.
2. Add a minimal CLI entrypoint with subcommands.
3. Define the adapter interface.
4. Implement the default adapter.
5. Add one real-world adapter for either `euresis` or `PhysEdit`.
6. Add a small fixtures-based test suite for:
   - entrypoint checks
   - task bundle creation
   - execution-context resolution

## Open Questions

- What implementation language gives the best tradeoff for local/remote distribution?
- How much machine-specific metadata should `Anton task` persist by default?
- Should repo adapters be code-only, file-based, or both?
- Should `anton threads` shell out to `codex-threads` directly first, or use a library boundary later?

## Constraint To Preserve

Do not expand Anton into a large orchestration runtime in the first implementation wave.

Keep it CLI-first, adapter-based, and tightly scoped to the recurring harness problems already observed.
