---
name: harness-audit
description: Run Anton doctor checks and interpret the repo-local Anton contract before deeper work.
---

# Harness Audit

Use this skill when a repo already adopts Anton and you need to understand whether
the current environment is safe to work in.

## Workflow

1. Verify the CLI is installed:
   - `command -v anton`
2. Read the repo contract:
   - `anton doctor --json`
3. Inspect the returned config source, entrypoint path, tasks root, and any degraded checks.
4. If the repo is not explicitly configured, note that Anton is running on built-in defaults.
5. Only move on to task or threads work after `doctor` is understood.

## Focus

- Treat `anton doctor --json` as the canonical execution/config receipt.
- Call out missing `anton.yaml`, missing entrypoint files, or missing `codex-threads`.
- Do not invent repo-specific runtime behavior; the repo should adapt through `anton.yaml`.

## Examples

```bash
anton doctor --json
```

```bash
anton doctor
```
