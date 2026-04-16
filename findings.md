# Findings

## Current Direction

- Anton should be a new repository.
- Anton should own reusable harness execution and validation surfaces.
- Anton v0 should center on:
  - doctor
  - task-state
  - threads
- execution-context resolution should first ship through `doctor --json`, not a separate v0 command
- `codex-threads` should remain separate and should not define Anton’s product identity.

## Proven Problems Driving This Repo

- `euresis` shows duplicated and drifting workflow entrypoints.
- Local and remote `PhysEdit` show heavy manual bootstrap and fragmented harness rules.
- Remote environments introduce PATH and filesystem-specific failures that repo-local docs do not absorb well.
- `codex-threads-insights` repeatedly shows `Context overload`, `Execution friction`, and weak closure evidence in active workflows.

## Product Boundary

Anton should own:

- task-state lifecycle and validation
- execution-context normalization
- local/remote doctor checks
- a canonical repo contract declared through `anton.yaml`
- a thin evidence-first threads surface

Anton should not own yet:

- orchestration daemon runtime
- job queueing
- deploy/PR automation
- full session indexing internals from `codex-threads`
- threads-centric workflow as the center of v0

## Contract Direction

- repos should adapt to Anton through `anton.yaml`
- canonical task bundles should live under `.anton/tasks/...`
- legacy repo layout migration is downstream adoption work, not Anton v0 runtime scope
