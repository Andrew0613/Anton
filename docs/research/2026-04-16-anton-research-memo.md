# Anton Research Memo

Date: 2026-04-16

## Decision

Create `Anton` as a new repository for a reusable harness CLI plus thin skill layer.

Do not overwrite or rename `codex-threads`.

## Why A New Repo

`codex-threads` already has a clear center of gravity:

- session indexing
- thread lookup
- event lookup
- recent-session aggregation
- insights generation
- evidence receipts

The current harness problem space is broader:

- generating and validating repo entrypoints
- bootstrapping and checking task-state bundles
- normalizing execution context across:
  - git worktrees
  - symphony-style directories
  - workspace-root non-git tasks
- running local/remote doctor checks
- smoothing over SSH, PATH, and filesystem differences

That broader scope justifies a separate repo.

## Evidence From Current Research

### Euresis

Observed problems:

- entrypoint duplication across `AGENTS.md`, `CLAUDE.md`, README, and workflow docs
- missing referenced docs in the copied harness map
- `project_progress` treated as a convention more than an enforced contract
- code-level harness is much thinner than the documented workflow
- runtime workspace behavior does not cleanly match the repo’s documented worktree contract

### PhysEdit

Observed problems:

- repeated cleanup tasks around workflow-doc drift and checker coverage
- highly manual complex-task bootstrap
- harness rules split across multiple AGENTS surfaces
- repo-specific scripts for task status, contract checks, grouped commit, and sync flows
- machine-readable `status.yaml` had to be added because markdown-only state was too brittle

### Remote Environment

Observed problems:

- non-interactive SSH sessions omit `~/.local/bin` from `PATH`
- remote `codex-threads` has confusing data-path vs source-path naming
- `cargo run` on shared mounts can fail with filesystem-specific Rust build errors
- explicit local-disk target-dir workarounds are needed on some hosts

### Usage Signals

`codex-threads-insights` evidence across recent sessions showed recurring:

- `Context overload`
- `Execution friction`
- `Low execution evidence`
- `Soft thread endings`

The strongest product implication is that Anton should optimize for:

- smaller evidence packs
- stronger closure receipts
- repeatable environment preflight
- stable execution contracts

## Official Guidance That Supports This Direction

### Anthropic

Recent Anthropic writing reinforces:

- harnesses should treat session and context management as first-class concerns
- long-running work benefits from explicit artifacts and handoffs
- parallel-agent systems should start from simpler coordination patterns
- tool surfaces should stay small and high-value
- managed-agent systems separate stable harness interfaces from underlying execution details

### OpenAI

Recent OpenAI writing reinforces:

- harness engineering is about environments, intent, and feedback loops
- repo knowledge should be the system of record
- short entrypoints should route into structured docs
- observability and evaluation need to be built into the toolchain, not bolted on later

## Repo Boundary

Anton should own:

- harness CLI
- entrypoint sync/check
- task-state init/pulse/check
- execution-context resolution
- remote/local doctor checks
- thin wrappers around `codex-threads`

`codex-threads` should own:

- indexing
- retrieval
- insight generation
- canonical evidence receipts

## Product Thesis

Anton is the reusable harness control plane.

`codex-threads` is one of Anton’s most important external dependencies.
