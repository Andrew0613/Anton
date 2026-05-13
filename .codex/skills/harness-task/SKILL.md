---
name: harness-task
description: Use Anton task-state and handoff commands to initialize, validate, pulse, and package canonical task bundles.
---

# Harness Task

Use this skill when working inside a repo that adopts Anton task bundles and you
need to update lifecycle state or prepare a handoff.

## Workflow

1. Verify the CLI is installed:
   - `command -v anton`
2. Confirm the repo contract:
   - `anton context --json`
   - `anton doctor --json`
3. Check the current task bundle:
   - `anton task-state check --json`
4. If the bundle does not exist yet, initialize it:
   - `anton task-state init --json`
5. After meaningful progress, refresh machine metadata:
   - `anton task-state pulse --json`
6. Use lifecycle commands only when they match the actual task state:
   - `anton task-state close --json`
   - `anton task-state reopen --json`
   - `anton task-state retarget --json`
   - `anton task-state import --json`
7. Before handing work to another agent or user, build a receipt:
   - `anton handoff build --json`

## Focus

- Canonical bundles live under `.anton/tasks/...` unless `anton.yaml` says otherwise.
- Task identity comes from `ANTON_TASK_ID`, the current branch, or the current bundle path.
- If task identity is missing, expect a structured `task-identity-required`
  failure rather than an auto-generated id.
- `task-state` remains the canonical lifecycle surface; do not assume a runtime
  `anton task` alias exists.
- Do not create alternate task-state layouts inside the repo.

## Examples

```bash
anton task-state check --json
```

```bash
anton task-state init --json
```

```bash
anton task-state pulse --json
```

```bash
anton handoff build --json
```
