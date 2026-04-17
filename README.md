# Anton

Reusable harness CLI for agentic repository workflows.

## Status

`Anton v0.0.2` is the current release.

The current surface is:

- `doctor`
- `task-state`
- `handoff`
- `threads`
- `version`

## Install

Build locally:

```bash
go build -o ./bin/anton ./cmd/anton
```

Install into `~/.local/bin`:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/anton ./cmd/anton
```

After the GitHub tag exists, install from source with:

```bash
go install github.com/Andrew0613/Anton/cmd/anton@v0.0.2
```

Check the installed version:

```bash
anton version
```

## Why This Repo Exists

Recent research across `euresis`, local/remote `PhysEdit`, workspace-level agent entrypoints, and `codex-threads-insights` showed recurring problems:

- entrypoint drift across `AGENTS.md`, `CLAUDE.md`, README blocks, and workflow docs
- task-state conventions that are only partially enforced
- runtime harness behavior that diverges from documented repo workflow
- local vs remote environment differences that break otherwise-valid commands
- repeated context overload and weak closure signals in recent sessions

Anton exists to turn those recurring harness problems into a reusable product surface.

## Core Positioning

Anton owns:

- harness doctor checks
- canonical task-state lifecycle
- repo-local harness contracts
- thin evidence-first threads integration

Anton does not own:

- orchestration daemons
- job queues
- PR/deploy automation
- `codex-threads` internals

`codex-threads` remains a separate tool with its own scope. Anton integrates
with it through a thin command surface, but Anton is defined first as an
independent harness.

## Canonical Repo Contract

Repos should adapt to Anton through a repo-local `anton.yaml`, not by expecting
Anton to grow repo-specific runtime logic.

The contract is intentionally bounded. Anton currently supports only:

- `version`
- `entrypoint.path`
- `tasks.root`
- `threads.default_project_strategy`
- `threads.workspace_roots`

Unknown fields are rejected so contract drift is explicit.

Canonical defaults:

- entrypoint: `AGENTS.md`
- task bundles: `.anton/tasks/active/<id_slug>/`
- optional thread workspaces: `.anton/workspaces/<project>/...`

Example `anton.yaml`:

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

Config source is always explicit in command output:

- `repo-local anton.yaml` when the file is present and valid
- `built-in defaults` when `anton.yaml` is missing

Task identity is inferred from:

- `ANTON_TASK_ID`
- a `task/.../<id_slug>` branch
- the current bundle path when already inside `.anton/tasks/...`

## Initial Scope

Released v0 modules:

- `doctor`
- `task-state`
- `handoff`
- `threads`
- `version`

Released v0 commands:

- `anton doctor`
- `anton task-state init`
- `anton task-state pulse`
- `anton task-state check`
- `anton task-state close`
- `anton task-state reopen`
- `anton task-state retarget`
- `anton task-state import`
- `anton handoff build`
- `anton threads doctor`
- `anton threads recent`
- `anton threads insights`
- `anton threads brief`
- `anton threads recipe`
- `anton version`

Planned later surfaces:

- `anton entrypoint check`
- `anton entrypoint sync`
- a dedicated `context resolve` surface if `doctor --json` proves too broad

`anton doctor --json` should emit the reusable execution contract for prompts and
handoffs, so Anton still owns context resolution without starting from a
separate command.

## Current Shape

The bootstrap implementation already follows the canonical contract:

- `anton doctor` resolves repo context, config source, entrypoint path, and
  execution risks
- `anton task-state` creates and checks canonical bundles under `.anton/tasks`
- `anton threads` stays thin and defers indexing/search semantics to
  `codex-threads`

## Quick Start

1. Add `anton.yaml` to the repo root.
2. Run `anton doctor --json`.
3. Create or validate a canonical task bundle with `anton task-state init --json`
   or `anton task-state check --json`.
4. Use `anton threads doctor --json` before relying on thread reads.

Minimal `anton.yaml`:

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

## CLI Reference

`anton doctor`

- checks writability, repo context, filesystem risk, configured entrypoint, `codex-threads`, and `anton.yaml`
- includes task identity drift checks and repo contract audit findings
- use `--json` for a stable execution/config receipt
- use `--explain` for remediation actions

`anton task-state init`

- creates `task_plan.md`, `findings.md`, `progress.md`, and `status.yaml`
- writes the bundle under `.anton/tasks/active/<id_slug>/` by default

`anton task-state check`

- validates required files and the current `status.yaml` schema

`anton task-state pulse`

- refreshes machine metadata and timestamps in `status.yaml`
- appends an execution attempt receipt

`anton task-state close`

- moves lifecycle into `blocked|review|partial|done` with closure gates
- requires stronger closure evidence for `done`

`anton task-state reopen`

- reopens lifecycle to `active` while preserving evidence history

`anton task-state retarget`

- renames the active bundle root and updates `stable.task_id`

`anton task-state import`

- imports external attempt/validation receipts into `status.yaml`

`anton handoff build`

- builds a compact handoff pack from task state and evidence receipts
- supports human-readable and `--json` outputs

`anton threads doctor`

- verifies the underlying `codex-threads` binary and returns its JSON doctor output

`anton threads recent`

- returns recent project-scoped threads when scope can be inferred

`anton threads insights`

- returns project-scoped aggregate thread signals

`anton threads brief`

- returns a thin scoped brief over `codex-threads threads recent`

`anton threads recipe`

- emits a reusable execution checklist over `codex-threads insights`

`anton version`

- prints the release version
- use `--json` for machine-readable output

## Repo-Local Skills

Anton `v0.0.2` includes three thin repo-local skills under `.codex/skills/`:

- `harness-audit`
- `harness-task`
- `harness-threads`

They are intentionally thin:

- `harness-audit` starts from `anton doctor --json`
- `harness-task` manages canonical task bundles through `anton task-state ...`
- `harness-threads` wraps `anton threads ...` without redefining `codex-threads`

If you want to use them outside this repo, copy or symlink these skill folders
into your shared Codex skills directory.

## Docs

- Research memo: [docs/research/2026-04-16-anton-research-memo.md](docs/research/2026-04-16-anton-research-memo.md)
- Requirements: [docs/plans/2026-04-16-anton-requirements.md](docs/plans/2026-04-16-anton-requirements.md)
- Implementation plan: [docs/plans/2026-04-16-anton-implementation-plan.md](docs/plans/2026-04-16-anton-implementation-plan.md)
- Handoff: [docs/handoffs/2026-04-16-anton-handoff.md](docs/handoffs/2026-04-16-anton-handoff.md)
- Changelog: [CHANGELOG.md](CHANGELOG.md)

## Development Notes

The first implementation pass should stay narrow:

- no orchestration daemon
- no PR/deploy automation
- no queueing/runtime scheduler
- no repo-specific runtime adapters
- no threads-centric product definition beyond a thin dependency surface

The right first milestone is a stable CLI that provides:

- environment clarity
- execution-contract clarity
- durable task state
- evidence-first thread lookup

across local repos and remote SSH environments.
