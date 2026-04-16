# Changelog

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
