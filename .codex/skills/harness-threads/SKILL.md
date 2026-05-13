---
name: harness-threads
description: Use Anton threads compatibility and native history receipts for evidence-first session context.
---

# Harness Threads

Use this skill when a repo already adopts Anton and you need recent thread,
insight, or local history evidence without remembering archive-reader details.

## Workflow

1. Verify both tools are available:
   - `command -v anton`
   - `anton threads doctor --json`
2. Let Anton infer project scope from flag, env, configured workspace roots, or repo root.
3. For compatibility with existing `codex-threads` evidence, start with recent threads:
   - `anton threads recent --json --limit 20`
4. Use insights for aggregate signals:
   - `anton threads insights --json --limit 50`
5. For vNext native local receipts, prefer bounded history commands:
   - `anton history show --json`
   - `anton history sync --json`
6. Use brief and recipe surfaces when you need a compact handoff-oriented view:
   - `anton threads brief --json`
   - `anton threads recipe --json`

## Focus

- `threads` remains a compatibility surface over `codex-threads`; native
  `history` is the vNext Anton-owned receipt surface.
- Prefer project-scoped reads. If Anton cannot infer a project, pay attention to the scope warning.
- Keep history receipts bounded and redacted; do not paste raw conversation or
  work-record payloads into downstream artifacts by default.
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

```bash
anton history sync --json
```
