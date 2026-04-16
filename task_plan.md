# Task Plan

## Goal

Turn the current harness research into a concrete starting point for `Anton`, a reusable harness CLI plus thin skill layer.

## Deliverables

- repository bootstrap
- research memo
- requirements doc
- implementation plan
- handoff doc for the next agent
- Go CLI skeleton
- minimal `doctor` command
- scaffolded `task-state` and `threads` command groups
- canonical repo-local `anton.yaml` contract

## Phases

- [x] Create the GitHub repo and local workspace.
- [x] Capture current research and repo boundary decisions.
- [x] Write Anton requirements.
- [x] Write the initial implementation plan.
- [x] Write a handoff document for a future implementation agent.
- [x] Align the docs around the `doctor` / `task-state` / `threads` v0 surface.
- [x] Bootstrap the Go CLI entrypoint and command router.
- [x] Implement a first pass of `anton doctor`.
- [x] Implement minimal usable `task-state` and `threads` command handlers.
- [x] Install or restore a working Go toolchain and run build/test validation.
- [x] Add a shared context/config layer and route the core commands through it.
- [x] Add fixture-based tests for repo context and task bundle scenarios.
- [x] Introduce the canonical `anton.yaml` contract and `.anton/tasks` layout.
- [x] Expose the resolved Anton config contract in `anton doctor`.
- [x] Remove repo-specific runtime adapter support from the main execution path.
- [x] Prepare a publishable `v0.0.1` release surface with versioning, README guidance, and thin skills.

## Notes

- Anton is a new repo, not a replacement for `codex-threads`.
- The first implementation wave should stay small and CLI-first.
- Repos should adapt to Anton through `anton.yaml`.
- Legacy layout migration is downstream repo adoption work, not Anton v0 scope.
- The first command groups are:
  - `doctor`
  - `task-state`
  - `threads`
