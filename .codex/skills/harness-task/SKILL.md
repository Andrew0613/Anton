---
name: harness-task
description: Use Anton task-state commands to initialize, validate, and pulse canonical task bundles.
---

# Harness Task

Use this skill when working inside a repo that adopts Anton task bundles.

## Workflow

1. Verify the CLI is installed:
   - `command -v anton`
2. Confirm the repo contract:
   - `anton doctor --json`
3. Check the current task bundle:
   - `anton task-state check --json`
4. If the bundle does not exist yet, initialize it:
   - `anton task-state init --json`
5. After meaningful progress, refresh machine metadata:
   - `anton task-state pulse --json`

## Focus

- Canonical bundles live under `.anton/tasks/...` unless `anton.yaml` says otherwise.
- Task identity comes from `ANTON_TASK_ID`, the current branch, or the current bundle path.
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
