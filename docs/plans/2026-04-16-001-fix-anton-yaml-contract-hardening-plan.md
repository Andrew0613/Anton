---
title: fix: Harden anton.yaml canonical contract
type: fix
status: completed
date: 2026-04-16
origin: docs/plans/2026-04-16-anton-requirements.md
---

# fix: Harden anton.yaml canonical contract

## Overview

Harden the minimal `anton.yaml` contract so `doctor`, `task-state`, and `threads` consistently consume one validated config path with explicit default-vs-repo reporting and stable invalid-config failures.

## Problem Frame

`PICA-231` is the root dependency for several high-priority Anton issues. The current config flow already supports canonical fields, but validation is permissive (unknown keys accepted, limited nested validation), default-versus-explicit policy reporting is coarse, and cross-command config contract visibility is inconsistent.

## Requirements Trace

- R1. Keep the `anton.yaml` surface minimal and explicitly bounded to canonical fields.
- R2. Report built-in defaults vs repo-local policy explicitly.
- R3. Fail invalid config clearly and consistently.
- R4. Ensure `doctor`, `task-state`, and `threads` consume the same resolved config contract.
- R5. Add test coverage for default, explicit override, and invalid config cases.

## Scope Boundaries

- No new top-level Anton commands.
- No repo-specific config adapters.
- No broad policy expansion beyond current canonical fields.

## Context & Research

### Relevant Code and Patterns

- `internal/adapter/config.go` already defines canonical fields and default overlay flow.
- `internal/adapter/adapter.go` is the shared resolution path used by all three command groups.
- `internal/doctor/doctor.go` already reports config source and path in JSON output.
- Existing fixture-based config tests live in `internal/adapter/adapter_test.go`.

### Institutional Learnings

- No `docs/solutions/` corpus exists yet in this repo; rely on current repo docs and tests.

### External References

- Not required for this pass; this issue is contract hardening inside an existing local architecture.

## Key Technical Decisions

- Use strict YAML decoding for config to reject unknown fields and reduce silent drift.
- Keep canonical field set unchanged; harden validation instead of expanding schema.
- Introduce shared config error typing/prefixing in adapter layer so command-level failures remain consistent while preserving existing command domains.
- Add compact config contract visibility to command JSON where missing, reusing the existing resolved config.

## Open Questions

### Resolved During Planning

- Should explicit `threads.default_project_strategy: ""` be accepted?
  - Resolution: no; missing field can inherit defaults, explicit empty value should fail validation.
- Should unknown config keys be tolerated?
  - Resolution: no; reject unknown keys in config decoding to keep contract bounded.

### Deferred to Implementation

- Exact JSON field naming for additional config metadata in `task-state`/`threads`.
  - Deferred reason: choose names after inspecting current JSON contract tests and keeping payload concise.

## Implementation Units

- [ ] **Unit 1: Harden config decoding and validation**

**Goal:** Make `anton.yaml` contract strict, bounded, and explicit on invalid policy.

**Requirements:** R1, R3

**Dependencies:** None

**Files:**
- Modify: `internal/adapter/status_yaml.go`
- Modify: `internal/adapter/config.go`
- Test: `internal/adapter/adapter_test.go`

**Approach:**
- Replace permissive config unmarshal with strict known-field decode for `anton.yaml`.
- Keep canonical field list unchanged but harden value validation:
  - explicit non-empty `entrypoint.path`
  - explicit non-empty `tasks.root`
  - `threads.default_project_strategy` in allowed enum only
  - reject empty workspace-root entries and obvious invalid root values.
- Preserve missing-file default overlay behavior.

**Patterns to follow:**
- Current config load/validate split in `internal/adapter/config.go`
- Existing fixture-oriented adapter tests in `internal/adapter/adapter_test.go`

**Test scenarios:**
- Happy path: valid explicit config loads and retains canonical values.
- Edge case: missing `anton.yaml` uses built-in defaults with `Loaded=false`.
- Error path: unknown field in YAML returns config parse/validation failure.
- Error path: unsupported `version`, empty `entrypoint.path`, empty `tasks.root`, invalid `threads.default_project_strategy`.
- Error path: invalid workspace root entry (empty string) fails validation.
- Integration: `adapter.Resolve` surfaces the same config validation failures for command consumers.

**Verification:**
- Adapter tests cover all validation branches and strict decode behavior.

- [ ] **Unit 2: Standardize resolved contract reporting across command surfaces**

**Goal:** Make default-vs-repo config policy explicit and consistently visible in machine-readable command outputs.

**Requirements:** R2, R4

**Dependencies:** Unit 1

**Files:**
- Modify: `internal/doctor/doctor.go`
- Modify: `internal/taskstate/taskstate.go`
- Modify: `internal/threads/threads.go`
- Test: `internal/doctor/doctor_test.go`
- Test: `internal/taskstate/taskstate_test.go`
- Test: `internal/threads/threads_test.go`

**Approach:**
- Reuse resolved adapter config in all three command families.
- Keep payloads concise but explicit:
  - config path
  - config source (`repo-local anton.yaml` vs built-in defaults)
  - key resolved contract values needed by downstream automation.
- Do not introduce a new command; extend current JSON payloads minimally.

**Execution note:** Start with failing JSON-output tests for each command family before implementation.

**Patterns to follow:**
- Existing doctor config contract payload in `internal/doctor/doctor.go`
- Existing command response envelope style in `taskstate` and `threads`

**Test scenarios:**
- Happy path: repo-local config is reported as repo-local source.
- Happy path: missing config reports built-in defaults source explicitly.
- Edge case: partial config still reports effective resolved values.
- Integration: `task-state` and `threads` JSON outputs include resolved config metadata from the same adapter resolution path as `doctor`.

**Verification:**
- JSON tests confirm explicit source reporting and consistent resolved values across all command families.

- [ ] **Unit 3: Stabilize invalid-config failure signaling and docs**

**Goal:** Ensure invalid `anton.yaml` errors are understandable and consistent, and document the bounded contract clearly.

**Requirements:** R1, R3, R5

**Dependencies:** Units 1-2

**Files:**
- Modify: `internal/adapter/config.go`
- Modify: `README.md`
- Test: `internal/doctor/doctor_test.go`
- Test: `internal/taskstate/taskstate_test.go`
- Test: `internal/threads/threads_test.go`

**Approach:**
- Wrap/normalize config-originated errors in adapter layer with stable message structure.
- Ensure each command returns domain-specific command error code but consistent config-failure message shape.
- Update README’s canonical contract section to reflect strict bounded behavior and explicit default-vs-config reporting.

**Patterns to follow:**
- Current command-level error envelopes (`ok/command/error`) across modules
- Existing README canonical contract section

**Test scenarios:**
- Happy path: valid config still returns success envelopes unchanged.
- Error path: invalid config produces stable and recognizable config error message across `doctor`, `task-state`, `threads`.
- Integration: same invalid config fixture triggers equivalent message prefix via all command entrypoints.

**Verification:**
- Command tests assert stable config-failure wording and non-zero exit behavior.
- README reflects effective canonical contract behavior.

## System-Wide Impact

- **Interaction graph:** `adapter.Resolve` remains single config ingress for `doctor`, `task-state`, `threads`.
- **Error propagation:** config decode/validation failures propagate from adapter into command envelopes without silent fallback.
- **State lifecycle risks:** none to task lifecycle semantics; only config load semantics and reporting.
- **API surface parity:** command JSON envelopes remain stable while adding minimal config metadata for parity.
- **Integration coverage:** one invalid-config fixture must be exercised through all three command families.
- **Unchanged invariants:** no new top-level commands; canonical field set remains `entrypoint.path`, `tasks.root`, `threads.default_project_strategy`, `threads.workspace_roots`.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Strict decode rejects existing repos with extra keys | Explicit error messaging and README contract clarification |
| JSON payload growth increases token usage | Keep additional config fields minimal and test payload size sanity |
| Inconsistent command-level error semantics | Centralize config error shaping in adapter layer and test all command entrypoints |

## Documentation / Operational Notes

- Update README contract section to state strict bounded fields and explicit default-vs-repo source reporting.
- No runtime rollout or migration required in this repo.

## Sources & References

- Origin document: `docs/plans/2026-04-16-anton-requirements.md`
- Related issue: `PICA-231`
- Related code: `internal/adapter/config.go`, `internal/doctor/doctor.go`, `internal/taskstate/taskstate.go`, `internal/threads/threads.go`
