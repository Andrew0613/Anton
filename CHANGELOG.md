# Changelog

## Unreleased

### Added

- add `anton workspace worktrees list|inspect|remove` for git worktree lifecycle management with safety classification, artifact detection, and dry-run removal
- add `file_contains_all` check kind to verify all required tokens are present in a file
- add `markdown_has_sections` check kind to verify required H2 headings across files matching a glob pattern
- add `run-events.jsonl` append-only event log written alongside `run.json` on each task status change
- add `anton run audit events` subcommand to read the event log
- add `--check` flag to `anton task list` for repair-class annotation (authority_mismatch, compatibility_drift, projection_semantics, freshness_stale)
- add `--freshness` / `--freshness --strict` flags to `anton task-state check` for freshness.status enforcement

### Changed

- extend `CheckSpec` with optional `Tokens []string`, `Sections []string`, and `PathPattern string` fields for the new check kinds

### Fixed

- serialize `anton run` manifest mutations so concurrent CLI writers do not
  lose checklist, audit, or close updates on the same task bundle

## v0.0.4 - 2026-05-20

### Added

- add passive `anton run` manifests for task-scoped checklist, audit, receipt,
  and close state
- add safe `anton gates run` execution for declared argv command gates with
  dry-run, profiles, timeouts, output caps, destructive-gate blocking, and
  optional run-manifest receipts
- add adopter harness inventory reports for classifying local harness surfaces
  during migration
- add run-manifest-only and heavy-harness fixtures plus a dogfood script for
  the consolidation workflow
- add migration guides for run manifests, safe gate execution, and heavy-harness
  consolidation

### Changed

- allow `task-state` to use `run_manifest` planning mode without requiring the
  planning-file triad
- include run manifest summaries and recent audit items in handoff output
- extend config, doctor, context, and contract projections with planning mode,
  run manifest, receipts directory, and gate profile fields
- clarify README and CONTEXT around passive run state, planning-file
  projections, and the no-daemon/no-backend boundary

### Fixed

- validate run manifests before they satisfy `task-state check`
- reject mismatched run manifest task IDs for active task operations
- keep attached gate receipts inside the task bundle and reject symlink receipt
  paths
- update gates warnings now that bounded command execution exists

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
