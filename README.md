# Anton

Anton is a small CLI for keeping agentic repository work grounded in one
machine-readable contract.

It helps a repo answer the questions that usually get scattered across
`AGENTS.md`, task notes, shell state, and past conversation history:

- What is the canonical entrypoint for agents?
- Where should task state live?
- Is this workspace safe to build, test, or hand off?
- Which task is active?
- Which local evidence should be preserved for the next agent?

Anton is intentionally boring infrastructure. It does not run agents, schedule
jobs, deploy code, or encode project-specific policy. Repos adapt to Anton with
`anton.yaml`; Anton keeps the contract stable.

The current harness consolidation work is adding passive run-manifest support:
Anton will record checklist state, attempts, audit notes, and validation
receipts around externally driven agent work. It is inspired by agent-runner
manifest, lifecycle, and audit ideas, but Anton is not becoming an agent runner
backend, daemon, scheduler, queue, or UI.

## Status

Current release: `v0.0.4`

The current checkout command families are:

- `doctor`
- `context`
- `preflight`
- `task-state`
- `run`
- `handoff`
- `threads`
- `adopt`
- `memory`
- `history`
- `gates`
- `entrypoint`
- `workspace`
- `migrate`
- `version`

Run manifests and the safe gates runner are passive sidecar surfaces. They
record externally driven work; they do not launch coding agents or introduce
backend scope.

## Install

Build from a local checkout:

```bash
go build -o ./bin/anton ./cmd/anton
```

Install into `~/.local/bin`:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/anton ./cmd/anton
```

After the GitHub tag exists, install from source:

```bash
go install github.com/Andrew0613/Anton/cmd/anton@v0.0.4
```

Check the installed version:

```bash
anton version
```

Anton currently expects the Go version declared by `go.mod`.

## Quick Start

Create `anton.yaml` in your repo root:

```yaml
version: 1
entrypoint:
  path: AGENTS.md
tasks:
  root: .anton/tasks
threads:
  default_project_strategy: repo-root
  workspace_roots:
    - .anton/workspaces
```

Inspect the repo contract:

```bash
anton context --json
```

Run start-work checks:

```bash
anton preflight --profile implementation --json
```

Run broader health checks:

```bash
anton doctor --json
```

Initialize or validate task state:

```bash
ANTON_TASK_ID=example anton task-state init --json
anton task-state check --json
```

Build a handoff receipt:

```bash
anton handoff build --json
```

## Configuration

Anton reads one repo-local config file: `anton.yaml`.

Supported fields:

- `version`
- `entrypoint.path`
- `tasks.root`
- `tasks.layout`
- `tasks.status_schema`
- `tasks.card_sync`
- `tasks.planning_mode`
- `run.enabled`
- `run.manifest`
- `run.receipts_dir`
- `gates`
- `gate_profiles`
- `threads.default_project_strategy`
- `threads.workspace_roots`

Unknown fields are rejected. This keeps contract drift explicit.

Default behavior:

- entrypoint: `AGENTS.md`
- task bundles: `.anton/tasks/active/<id_slug>/`
- task layout: `anton`
- status schema: `anton`
- planning mode: `planning_files`
- run manifest: `run.json`
- run receipts: `receipts`
- optional thread workspaces: `.anton/workspaces/<project>/...`

Config source is always shown in command output:

- `repo-local anton.yaml` when a valid file is present
- `built-in defaults` when `anton.yaml` is missing

Task identity is inferred from:

- `ANTON_TASK_ID`
- a `task/.../<id_slug>` branch
- the current bundle path when already inside `.anton/tasks/...`

Anton does not invent a task id when identity is missing. Commands that need one
return a structured `task-identity-required` error.

## Core Concepts

### Contract

`anton context --json` emits the compact `ContractV1` receipt for the current
workspace.

`anton doctor --json` uses the same contract and adds health checks,
remediation, and execution-risk findings.

Use these commands at the start of agent work, before trusting local state.

### Preflight

`anton preflight --profile investigation --json` is a read-only check for
research and inspection work.

`anton preflight --profile implementation --json` is stricter: missing task
identity is blocked so coding agents do not begin writes without a task record.

### Task State

`anton task-state` owns the canonical task lifecycle.

It creates and checks bundles under `.anton/tasks` by default. A bundle contains:

- `task_plan.md`
- `findings.md`
- `progress.md`
- `status.yaml`

Use `task-state pulse` after meaningful progress. Use `handoff build` before
passing work to another person or agent.

The `task_plan.md`, `findings.md`, and `progress.md` files are compatibility and
projection artifacts for repos that still use planning-file workflows. New
adopters can use Anton-native task state plus the run manifest for
machine-checkable work state.

### Run Manifest

`anton run` owns passive execution state for the active task: checklist items,
audit notes, validation receipts, attempts, and close state. It requires an
existing task bundle and never creates or mutates task lifecycle state.

```bash
ANTON_TASK_ID=example anton task-state init --json
ANTON_TASK_ID=example anton run init --json
ANTON_TASK_ID=example anton run task add --id u1 --title "Run tests" --json
```

Repos with topic-layer task bundles can declare them without repo-specific
Anton code:

```yaml
tasks:
  root: project_progress
  layout: topic-layer
```

Non-native status schemas must be declared explicitly, for example
`status_schema: physedit-v1`. Anton can read compatible summaries through the
adapter, but lifecycle mutation commands are only enabled for the native Anton
status schema until a schema-aware mutation contract lands.

### Evidence

Anton has two evidence surfaces:

- `threads`: compatibility wrapper around `codex-threads`
- `history`: Anton-native local receipts under `.anton/history/receipts.jsonl`

`threads` remains useful when `codex-threads` is installed. `history` is the
native Anton surface for bounded local evidence.

### Gates

`gates list` and `gates check` read declarative gate metadata. `gates run` has a
bounded execution subset for declared argv-style command gates. It enforces a
repo-contained cwd, timeouts, output caps, destructive-gate blocking by default,
and optional `--attach-run` receipts that append to the run manifest.

## Command Reference

### Contract and Health

```bash
anton context [--json|--explain]
anton preflight --profile investigation [--json]
anton preflight --profile implementation [--json]
anton doctor [--json|--explain]
anton entrypoint check [--json]
```

Use these to resolve the current repo contract, entrypoint, environment, and
execution risks.

### Task Lifecycle

```bash
anton task-state init [--json]
anton task-state pulse [--json]
anton task-state check [--schema anton|auto|physedit-v1] [--json]
anton task-state env [--json]
anton task-state service add [--json]
anton task-state freshness [--json]
anton task-state sync-card [--json]
anton task-state close [--json]
anton task-state reopen [--json]
anton task-state retarget [--json]
anton task-state import [--json]
anton handoff build [--source manual|codex|claude] [--session-id ID] [--json]
anton handoff persist-results --worktree-root PATH --run-dir PATH --dry-run [--json]
```

Use these to create, validate, update, close, reopen, retarget, import, and hand
off canonical task bundles.

### Run State

```bash
anton run init [--json]
anton run status [--json]
anton run task list [--json]
anton run task add --id ID --title TITLE [--json]
anton run task set --id ID --status pending|in_progress|blocked|done|dropped [--note NOTE] [--json]
anton run audit add --kind KIND --name NAME --status STATUS [--summary SUMMARY] [--receipt-path PATH] [--json]
anton run close --status open|review|done|blocked|canceled [--summary SUMMARY] [--json]
```

Use these to manage passive run manifests under the active task bundle.

### Evidence and Memory

```bash
anton threads doctor [--json]
anton threads recent [--json]
anton threads insights [--json]
anton threads brief [--json]
anton threads recipe [--json]
anton history show [--json]
anton history sync [--json]
anton memory status [--json]
anton memory update [--json]
```

Use `threads` for `codex-threads` compatibility. Use `history` and `memory` for
Anton-native repo-local receipts.

### Adoption and Maintenance

```bash
anton adopt plan [--json]
anton adopt harness-inventory [--json|--format markdown]
anton gates list [--json]
anton gates check [--json]
anton gates run [--gate NAME|--profile NAME] [--dry-run] [--attach-run] [--json]
anton workspace inspect [--json]
anton workspace check [--json]
anton workspace refs --target PATH [--json]
anton migrate plan [--json]
anton migrate readiness --target PATH [--json]
```

These commands are read-only or append-only where noted. `migrate plan` is
currently blocked until the next config schema is locked, and
`migrate readiness` never moves files.

### Version

```bash
anton version [--json]
```

## Repo-Local Skills

Anton includes three thin Codex skills under `.codex/skills/`:

- `harness-audit`
- `harness-task`
- `harness-threads`

They are wrappers around Anton commands, not separate policy engines.

Use them as examples for downstream repos that want agent-facing workflows over
the same CLI contract.

## Design Boundaries

Anton owns these passive CLI-sidecar surfaces:

- repo contract inspection
- entrypoint checks
- task-state lifecycle
- passive run manifests and audit receipts
- handoff receipts
- local evidence receipts
- read-only workspace checks
- declared gate metadata and bounded gate receipts

Anton does not own:

- agent orchestration
- coding-agent backends
- daemons or schedulers
- job queues
- product UI
- deployment automation
- PR automation
- repo-specific business logic

Project-specific conventions should live in the repo and be declared through
config or documented entrypoints.

## Development

Run tests:

```bash
TMPDIR=/var/tmp ~/.local/share/go1.22.0/bin/go test ./...
```

Build locally:

```bash
go build -o /tmp/anton ./cmd/anton
```

Run a smoke check from this repo:

```bash
/tmp/anton doctor --json
```

Run a harness-consolidation dogfood loop:

```bash
scripts/dogfood_harness_consolidation.sh
```

## Documentation

- [Research memo](docs/research/2026-04-16-anton-research-memo.md)
- [Requirements](docs/plans/2026-04-16-anton-requirements.md)
- [Implementation plan](docs/plans/2026-04-16-anton-implementation-plan.md)
- [Harness consolidation guide](docs/guides/harness-consolidation.md)
- [Run manifest guide](docs/guides/run-manifest.md)
- [Gates runner guide](docs/guides/gates-runner.md)
- [Handoff](docs/handoffs/2026-04-16-anton-handoff.md)
- [Changelog](CHANGELOG.md)
