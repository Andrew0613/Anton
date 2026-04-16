# Anton Implementation Plan

Date: 2026-04-16

## Strategy

Build Anton as a narrow CLI first.

Do not start with subagents, daemon orchestration, or a broad plugin ecosystem.

The first implementation should prove that Anton can unify repeated harness pain in a way that is smaller and more stable than the current repo-by-repo scripts.

## Phase 0: Bootstrap

- choose implementation language and packaging
- add README, AGENTS, and core docs
- define CLI command tree
- define adapter interface

Preferred bias:

- use a language that is easy to distribute and script from local and remote machines
- optimize for stable CLI behavior over early framework sophistication

## Phase 1: Doctor + Context

Implement:

- `anton doctor`
- `anton doctor repo`
- `anton doctor remote`
- `anton context resolve`

Why first:

- this gives immediate value before the rest of the system exists
- it directly attacks current environment and drift problems

## Phase 2: Task State

Implement:

- `anton task init`
- `anton task pulse`
- `anton task check`

Deliverables:

- generic task bundle schema
- repo adapter hooks for layout differences
- normalized execution metadata

## Phase 3: Entry Point Sync

Implement:

- `anton entrypoint check`
- `anton entrypoint sync`

Deliverables:

- short template system
- required-reference checks
- line-budget checks
- missing-file detection

## Phase 4: Threads Adapter

Implement:

- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`

Deliverables:

- local/remote invocation resolver
- binary-path discovery
- source-dir vs data-dir disambiguation
- optional Rust target-dir workaround support

## Phase 5: Thin Skill Layer

After the CLI is usable, add thin skills that wrap it:

- `harness-audit`
- `harness-task`
- `harness-threads`

These skills should stay thin and mostly assemble the right CLI invocations plus repo adapter choices.

## Risks

- overbuilding before real command usage exists
- letting `Anton` slowly absorb repo-specific logic
- duplicating too much of `codex-threads`
- choosing a runtime that is painful to distribute on remote hosts

## Guardrails

- every command should solve a real pain already observed in research
- keep the default adapter generic
- prefer composition over deep integration with one repo
- do not add orchestration runtime features until the CLI surfaces are stable
