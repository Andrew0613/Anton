# Changelog

## v0.0.2 - 2026-04-17

- add lifecycle task-state operations: `close`, `reopen`, `retarget`, `import`
- add `handoff build` with machine-readable task/evidence packs
- extend `threads` with `brief` and `recipe` thin wrappers
- harden task id safety to block path traversal in bundle and retarget flows
- preserve lifecycle semantics on `task-state pulse` (no implicit reopen)
- strengthen doctor preflight with remediation and contract-drift signals

## v0.0.1 - 2026-04-16

- bootstrap Go CLI with `doctor`, `task-state`, `threads`, and `version`
- define the canonical repo contract through `anton.yaml`
- standardize task bundles under `.anton/tasks/...`
- expose execution/config receipts through `anton doctor --json`
- keep `threads` as a thin wrapper around `codex-threads`
- add repo-local thin skills:
  - `harness-audit`
  - `harness-task`
  - `harness-threads`
