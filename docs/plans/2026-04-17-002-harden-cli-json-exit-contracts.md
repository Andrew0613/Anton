---
title: fix: Harden machine-readable CLI contracts
type: fix
status: completed
date: 2026-04-17
origin: https://linear.app/pica-research/issue/PICA-238
---

# fix: Harden machine-readable CLI contracts

## Overview

Lock Anton's current machine-readable contract for `doctor`, `task-state`, and `threads` with golden JSON and explicit exit-code expectations so downstream automation can detect intentional contract changes.

## Requirements Trace

- R1. Golden/fixture-backed JSON contract tests for `anton doctor --json`, `anton task-state ... --json`, `anton threads ... --json`.
- R2. Explicit exit-code assertions for usage errors, degraded/blocked states, and execution failures.
- R3. Keep human-readable output free to evolve while machine-readable output remains stable.
- R4. Preserve current v0 command surface (no new top-level commands).

## Scope Boundaries

- No response versioning in this pass.
- No repo-specific adapters.
- No command-surface expansion.

## Implementation Units

- [x] **Unit 1: Introduce reusable golden JSON helpers for command contract tests**

Goal: Make schema drift obvious in review with stable golden fixtures and shared assertion helpers.

Requirements: R1, R3

Files:
- Add: `internal/doctor/testdata/golden/*.json`
- Add: `internal/taskstate/testdata/golden/*.json`
- Add: `internal/threads/testdata/golden/*.json`
- Modify: `internal/doctor/doctor_test.go`
- Modify: `internal/taskstate/taskstate_test.go`
- Modify: `internal/threads/threads_test.go`

Execution note: characterization-first.

Approach:
- Add minimal helper functions to normalize dynamic paths before comparison.
- Compare produced JSON against checked-in golden files using structural equality.
- Keep helper shape local to each package unless cross-package reuse is clearly simpler.

Verification:
- Tests fail with clear diffs when JSON contract changes.

- [x] **Unit 2: Expand doctor contract and exit-code coverage**

Goal: Pin stable JSON output and exit behavior for success/degraded/failure/usage paths.

Requirements: R1, R2

Dependencies: Unit 1

Files:
- Modify: `internal/doctor/doctor_test.go`
- Add: `internal/doctor/testdata/golden/*.json`

Execution note: test-first.

Approach:
- Add golden coverage for representative `doctor --json` output.
- Add explicit exit-code tests for:
  - usage (`unexpected argument`) => `2`
  - degraded checks => `1`
  - config/runtime failure => `1`
- Ensure no stderr for JSON error payload path.

Verification:
- `go test ./internal/doctor` passes with contract and exit-code assertions.

- [x] **Unit 3: Expand task-state contract and exit-code coverage**

Goal: Pin stable JSON output and exit behavior for `init`, `check`, `pulse`.

Requirements: R1, R2

Dependencies: Unit 1

Files:
- Modify: `internal/taskstate/taskstate_test.go`
- Add: `internal/taskstate/testdata/golden/*.json`

Execution note: test-first.

Approach:
- Add golden JSON cases for representative blocked and successful flows.
- Add explicit exit-code tests for:
  - usage errors => `2`
  - blocked `check`/`pulse` => `1`
  - runtime failures => `1`
- Keep assertions centered on machine-readable payload contract.

Verification:
- `go test ./internal/taskstate` passes with golden + exit-code coverage.

- [x] **Unit 4: Expand threads contract and exit-code coverage**

Goal: Pin stable JSON output and exit behavior for `doctor`, `recent`, `insights`.

Requirements: R1, R2

Dependencies: Unit 1

Files:
- Modify: `internal/threads/threads_test.go`
- Add: `internal/threads/testdata/golden/*.json`

Execution note: test-first.

Approach:
- Reuse fake `codex-threads` binary fixtures to produce deterministic JSON payloads.
- Add golden assertions for wrapped adapter metadata + raw payload.
- Add explicit exit-code tests for usage and binary/runtime failure paths.

Verification:
- `go test ./internal/threads` passes with stable JSON/exit assertions.

- [x] **Unit 5: End-to-end verification and issue closure**

Goal: Validate repo-wide quality gates and update Linear with explicit shipped contract scope.

Requirements: R1, R2, R3, R4

Dependencies: Units 2-4

Files:
- Modify (if needed): `README.md`

Execution note: pragmatic.

Approach:
- Run targeted tests, then full `go test ./...`.
- Update docs only if machine-readable contract guidance needs clarification.
- Post implementation summary and test evidence to Linear issue.

Verification:
- Full test suite passes and issue has implementation evidence.
