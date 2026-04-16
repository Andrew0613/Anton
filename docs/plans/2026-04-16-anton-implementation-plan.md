# Anton Implementation Plan

Date: 2026-04-16

## Strategy

Build Anton as a narrow CLI first.

Do not start with subagents, daemon orchestration, or a broad plugin ecosystem.

The first implementation should prove that Anton can unify repeated harness pain in a way that is smaller and more stable than the current repo-by-repo scripts.

## Phase 0: Bootstrap

- use Go for the CLI
- add README, AGENTS, and core docs
- define CLI command tree
- define the canonical Anton repo contract
- load repo-local `anton.yaml`

Language decision:

- Anton v0 uses Go
- optimize for stable CLI behavior, easy local/remote distribution, and simple system integration

## Phase 1: Doctor

Implement:

- `anton doctor`

Why first:

- this gives immediate value before the rest of the system exists
- it directly attacks current environment and drift problems
- it can carry the execution-contract output without opening a separate command yet

## Phase 2: Task State

Implement:

- `anton task-state init`
- `anton task-state pulse`
- `anton task-state check`

Deliverables:

- generic task bundle schema
- canonical `.anton/tasks` layout
- repo-local config for task-root overrides
- normalized execution metadata

## Phase 3: Threads

Implement:

- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`

Deliverables:

- thin `codex-threads` invocation layer
- project-scoped evidence-first defaults
- config-driven workspace-root inference
- path/discovery checks for local vs remote hosts

## Phase 4: Thin Skill Layer

Implement:

- thin wrappers around doctor/task-state/threads

Deliverables:

- `harness-audit`
- `harness-task`
- `harness-threads`

These skills should stay thin and mostly assemble the right CLI invocations plus
repo-local Anton config.

## Phase 5: Entry Point Sync

Implement later:

- `anton entrypoint check`
- `anton entrypoint sync`

Why later:

- important, but not part of the smallest useful harness core
- depends on a stable canonical repo contract first

## Phase 6: Optional External Integrations

Evaluate later:

- other external capability integrations

These should be additive integrations, not the defining center of Anton. `codex-threads`
already has a thin first-party dependency surface in Phase 3.

## Risks

- overbuilding before real command usage exists
- letting `Anton` slowly absorb repo-specific runtime branches
- letting the `threads` surface redefine Anton as an external-tool wrapper instead of a harness
- choosing a runtime that is painful to distribute on remote hosts

## Guardrails

- every command should solve a real pain already observed in research
- keep one canonical runtime contract
- prefer repo-local config over repo-specific code paths
- do not add orchestration runtime features until the CLI surfaces are stable
- keep Anton v0 centered on doctor/task-state/threads
