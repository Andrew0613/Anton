# Anton Requirements

Date: 2026-04-16

## Summary

Anton is a reusable harness CLI and thin skill layer for agentic repository workflows.

It should make current cross-repo harness patterns executable and checkable without forcing each repo to reinvent the same scripts and docs.

## Goals

- unify repeated harness behaviors across repos
- reduce workflow drift between docs and implementation
- make task-state lifecycle machine-checkable
- normalize execution context across local and remote environments
- give repos one explicit way to adapt to Anton through config
- expose a stable evidence-first threads surface without redefining Anton around it

## Non-Goals

- not a general-purpose orchestration daemon
- not a PR/deploy platform
- not a wrapper-first UX for `codex-threads`
- not a monolithic “agent operating system”

## Primary Users

- a human developer working across several repos
- an implementation agent bootstrapping a complex task
- a research/planning agent that needs stable receipts and context resolution

## Core Use Cases

1. Bootstrap a new task bundle in a consistent shape.
2. Render or check current task state.
3. Render a current execution contract for:
   - git worktree
   - repo root
   - workspace root
   - remote SSH target
4. Run stable harness doctor checks for local and remote execution surfaces.
5. Read recent thread and evidence signals through one stable command surface.

## v0 Command Surface

### Doctor

- `anton doctor`

### Task State

- `anton task-state init`
- `anton task-state pulse`
- `anton task-state check`

### Threads

- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`

## Required v0 Behaviors

### Task State

- create required task bundle files
- validate presence and basic schema
- normalize machine metadata carefully
- separate stable task identity from volatile machine-specific details
- default to one canonical bundle layout

### Context Resolution

- identify whether current execution target is:
  - git worktree
  - plain repo root
  - workspace-root non-git context
  - remote SSH target
- emit a standard contract that can be pasted into agent prompts
- make that contract available from `anton doctor --json`

### Doctor

- check whether the current repo and execution target satisfy minimum harness assumptions
- flag remote reachability and obvious environment risks
- flag missing writable paths or missing task-state prerequisites
- expose the resolved Anton config contract in `doctor --json`
- stay focused on harness health rather than external analytics tooling

### Threads Adapter

- discover a usable `codex-threads` invocation strategy
- prefer on-PATH execution when available
- detect common drift like “binary exists but is not on PATH”
- keep output evidence-first and project-scoped by default

## Repo Contract Model

Anton runtime should stay canonical and config-driven.

A repo adapts to Anton by providing `anton.yaml`.

`anton.yaml` can declare:

- `entrypoint.path`
- `tasks.root`
- `threads.default_project_strategy`
- `threads.workspace_roots`

Anton should not grow repo-specific runtime adapters for `euresis`, `PhysEdit`,
or other downstream repos.

Legacy repo layout migration is downstream adoption work. Anton v0 should not
branch its runtime or roadmap around helping each existing repo migrate.

## Success Criteria

Anton v0 is successful if it can:

1. bootstrap a task bundle in a clean new repo
2. resolve execution context for local and remote scenarios
3. run useful doctor checks across local and remote scenarios
4. load a repo-local `anton.yaml` contract and reflect it in `doctor --json`
5. run at least one stable threads workflow through a thin dependency surface
6. reduce at least one current manual workflow into a single stable command

## Deferred Work

Explicitly deferred beyond v0:

- `anton entrypoint check`
- `anton entrypoint sync`
- a dedicated `anton context resolve` command if `doctor --json` is not enough
- deeper integrations with external tools beyond the thin `threads` surface
