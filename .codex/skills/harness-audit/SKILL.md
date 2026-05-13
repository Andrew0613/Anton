---
name: harness-audit
description: Run Anton contract, doctor, entrypoint, workspace, and declarative gate checks before deeper work.
---

# Harness Audit

Use this skill when a repo already adopts Anton and you need to understand
whether the current environment and repo contract are safe to work in.

## Workflow

1. Verify the CLI is installed:
   - `command -v anton`
2. Read the compact repo contract:
   - `anton context --json`
3. Run the health-oriented doctor receipt:
   - `anton doctor --json`
4. Inspect the returned config source, entrypoint path, task identity, tasks root,
   prompt contract, and any degraded checks.
5. If the repo is not explicitly configured, note that Anton is running on
   built-in defaults.
6. For vNext repos, check the surrounding contract surfaces as needed:
   - `anton entrypoint check --json`
   - `anton workspace inspect --json`
   - `anton workspace check --json`
   - `anton gates list --json`
   - `anton gates check --json`
7. Only move on to task, handoff, history, or threads work after the contract
   and health receipts are understood.

## Focus

- Treat `anton context --json` and `anton doctor --json` as projections over the
  same canonical contract, with `doctor` adding health and remediation detail.
- Call out missing `anton.yaml`, missing entrypoint files, or missing `codex-threads`.
- Keep extension fields advisory unless a command explicitly declares authority
  for them.
- Do not invent repo-specific runtime behavior; the repo should adapt through `anton.yaml`.

## Examples

```bash
anton context --json
```

```bash
anton doctor --json
```

```bash
anton entrypoint check --json
```

```bash
anton gates check --json
```
