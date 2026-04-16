---
name: harness-threads
description: Use Anton as a thin evidence-first wrapper around codex-threads.
---

# Harness Threads

Use this skill when a repo already adopts Anton and you need recent thread or
insight data without remembering local `codex-threads` conventions.

## Workflow

1. Verify both tools are available:
   - `command -v anton`
   - `anton threads doctor --json`
2. Let Anton infer project scope from flag, env, configured workspace roots, or repo root.
3. Start with recent threads:
   - `anton threads recent --json --limit 20`
4. Use insights for aggregate signals:
   - `anton threads insights --json --limit 50`

## Focus

- Anton should stay thin here; `codex-threads` remains the underlying evidence system.
- Prefer project-scoped reads. If Anton cannot infer a project, pay attention to the scope warning.
- Do not add repo-specific threads logic to Anton runtime.

## Examples

```bash
anton threads doctor --json
```

```bash
anton threads recent --json --limit 10
```

```bash
anton threads insights --json --limit 25
```
