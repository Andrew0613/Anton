# Changelog

## Unreleased

- add `preflight` investigation and implementation profiles for read-only
  start-work checks
- add topic-layer task-state support through `tasks.layout`,
  `tasks.status_schema`, and related lifecycle metadata commands
- add source-aware handoff packets plus dry-run result persistence planning
- add read-only `workspace refs` and `migrate readiness` reports for path moves
- improve declarative gate summaries while keeping `gates run` blocked

## v0.0.3 - 2026-05-08

- add vNext command surfaces: `context`, `adopt`, `memory`, `history`, `gates`,
  `entrypoint`, `workspace`, and `migrate`
- make `doctor` and `context` share the canonical contract projection
- preserve `task-state` as the canonical lifecycle surface while deferring the
  runtime `task` alias
- add native history and memory receipts alongside the existing `threads`
  compatibility surface
- add declarative gate, entrypoint, workspace, and migration preview checks
- update repo-local harness skills for the vNext workflow
- harden `doctor` to flag Go toolchains older than the repo `go.mod` directive
- improve agent-facing CLI ergonomics with global `--json`, leaf `--help`, and
  degraded `doctor --json` responses that agree with their non-zero exit code

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
