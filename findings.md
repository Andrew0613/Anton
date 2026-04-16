# Findings

## Current Direction

- Anton should be a new repository.
- Anton should own reusable harness execution and validation surfaces.
- `codex-threads` should remain the memory/trace/insight backend and adapter target.

## Proven Problems Driving This Repo

- `euresis` shows duplicated and drifting workflow entrypoints.
- Local and remote `PhysEdit` show heavy manual bootstrap and fragmented harness rules.
- Remote environments introduce PATH and filesystem-specific failures that repo-local docs do not absorb well.
- `codex-threads-insights` repeatedly shows `Context overload`, `Execution friction`, and weak closure evidence in active workflows.

## Product Boundary

Anton should own:

- entrypoint generation and checks
- task-state lifecycle and validation
- execution-context normalization
- local/remote doctor checks
- `codex-threads` adapter commands

Anton should not own yet:

- orchestration daemon runtime
- job queueing
- deploy/PR automation
- full session indexing internals from `codex-threads`
