# Harness Consolidation Guide

This guide is for repos with heavy local harness code: long entrypoints, planning
file requirements, task-state scripts, validation wrappers, and handoff helpers.
The migration goal is to move generic harness behavior into Anton while keeping
project-specific policy in the adopter repo.

Anton remains a CLI-first sidecar. It records state, checks readiness, and writes
receipts around externally driven work. It does not run coding agents, own a
daemon, schedule jobs, manage queues, provide a UI, or replace a project's build
or deployment system.

## Migration Sequence

1. Add or update `anton.yaml`.

   Declare the repo entrypoint, task root, task layout, thread workspace roots,
   and any future run or gate settings through config. Avoid adding adopter
   names or research policy to Anton core.

2. Initialize task state.

   Use `anton task-state init --json` once task identity is available. Task
   lifecycle remains owned by `task-state`: identity, status, freshness, close,
   reopen, retarget, and handoff readiness stay there.

3. Initialize the run manifest.

   Once the run surface lands, use `anton run init --json` to create passive
   execution state for checklist items, attempts, audit notes, and receipts. The
   run manifest complements task lifecycle state; it does not replace task
   identity or lifecycle ownership.

4. Move planning-file hard rules to projection mode.

   Repos that previously required `planning-with-files` should stop treating
   `task_plan.md`, `findings.md`, and `progress.md` as the canonical harness
   contract. Keep them during migration as compatibility or review projections,
   then prefer Anton-native task state plus run manifest state for new tasks.

5. Replace local wrapper checks with declared gates.

   Move generic validation commands into declared gate metadata. The planned
   `anton gates run` surface is only for explicit argv-style commands with
   bounded cwd, timeout, output caps, exit-code capture, and receipts. Keep shell
   pipelines, destructive operations, and project-specific infra policy outside
   Anton core unless a separate reviewed safety contract exists.

6. Use adopter inventory to classify local harness files.

   The intended `anton adopt harness-inventory` report should classify files as
   `move_to_anton`, `keep_project_local`, `delete_or_deprecate`, or
   `manual_review`. Treat this as a migration aid, not an automatic deleter.

7. Produce handoff from task state plus run receipts.

   `handoff` should summarize lifecycle state, run checklist progress, recent
   audit notes, and gate receipts. It should still degrade gracefully for repos
   that only have legacy planning files.

## For Repos Using Planning-With-Files

Do not delete planning files first. Start in a hybrid mode:

```yaml
tasks:
  root: .anton/tasks
  planning_mode: hybrid
run:
  enabled: true
  manifest: run.json
  receipts_dir: receipts
```

During migration, keep existing planning files readable for agents and reviewers
that still expect them. New work should update Anton-native state first, then
emit or maintain planning files only as projections. After the run manifest path
is proven on active tasks, switch new adopters to:

```yaml
tasks:
  planning_mode: run_manifest
```

Legacy repos can remain on `planning_files` until they have equivalent Anton
receipts.

## What To Keep Project-Local

Keep project-specific research policy, dataset rules, benchmark thresholds,
cluster assumptions, credential handling, and domain-specific triage in the
adopter repo. Anton may record the result of those checks, but it should not
learn the project's business logic.

Keep destructive commands behind local project review. A gate receipt is useful
only when the command policy is explicit and reversible enough for the repo.

## What Not To Migrate Into Anton Core

Do not migrate:

- coding-agent backend launchers
- daemon or scheduler control loops
- job queues or budget systems
- product UI or dashboards
- project-specific branch, task, dataset, or path names
- arbitrary shell wrappers
- deployment or PR automation

Anton should stay the reusable harness substrate: contract, state, checks,
receipts, and handoff.
