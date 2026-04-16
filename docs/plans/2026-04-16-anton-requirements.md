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
- make `codex-threads` easier to use consistently across hosts and repos

## Non-Goals

- not a general-purpose orchestration daemon
- not a PR/deploy platform
- not a replacement for `codex-threads`
- not a monolithic “agent operating system”

## Primary Users

- a human developer working across several repos
- an implementation agent bootstrapping a complex task
- a research/planning agent that needs stable receipts and context resolution

## Core Use Cases

1. Validate a repo’s entrypoint contract.
2. Bootstrap a new task bundle in a consistent shape.
3. Render or check current task state.
4. Resolve the current execution context for:
   - git worktree
   - repo root
   - workspace root
   - remote SSH target
5. Run a stable `threads` surface without caring whether:
   - `codex-threads` is on `PATH`
   - the host needs an absolute binary path
   - the source repo and data directory differ
   - a shared mount needs a different Rust target dir

## v0 Command Surface

### Doctor

- `anton doctor`
- `anton doctor repo`
- `anton doctor remote`
- `anton doctor threads`

### Task State

- `anton task init`
- `anton task pulse`
- `anton task check`

### Entrypoints

- `anton entrypoint check`
- `anton entrypoint sync`

### Context

- `anton context resolve`

### Threads

- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`

## Required v0 Behaviors

### Entry Points

- check file existence
- check line budgets
- check required references
- flag broken relative doc links
- flag drift between generated and current content

### Task State

- create required task bundle files
- validate presence and basic schema
- normalize machine metadata carefully
- separate stable task identity from volatile machine-specific details

### Context Resolution

- identify whether current execution target is:
  - git worktree
  - plain repo root
  - workspace-root non-git context
  - remote SSH target
- emit a standard contract that can be pasted into agent prompts

### Threads Adapter

- discover usable `codex-threads` invocation strategy
- prefer direct binary use when available
- support explicit source-dir execution when needed
- support host-specific workarounds like local target dirs

## Adapter Model

Anton should support repo adapters.

Each adapter can declare:

- repo name
- task bundle layout
- entrypoint templates
- validation hooks
- context rules
- optional thread project mapping

The default adapter should be small and generic.

`euresis` and `PhysEdit` should become explicit adapters rather than hardcoded assumptions.

## Success Criteria

Anton v0 is successful if it can:

1. bootstrap a task bundle in a clean new repo
2. check and sync entrypoint docs in at least one real repo
3. resolve execution context for local and remote scenarios
4. successfully proxy at least one `codex-threads` workflow across local and remote hosts
5. reduce at least one current manual workflow into a single stable command
