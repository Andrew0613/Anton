---
title: feat: Anton vNext confidence lock
type: feat
status: completed
date: 2026-05-08
origin: docs/plans/2026-05-08-001-feat-anton-vnext-master-roadmap-plan.md
---

# feat: Anton vNext confidence lock

## Overview

This plan closes the strategy loopholes found during the confidence review of
the Anton vNext plan family. It is not another feature slice. It is the
merge-blocking checklist that decides whether an implementation agent may start
Slice 1 and whether future surface plans may graduate from roadmap status.

## Confidence Claim

The strategy is considered implementation-ready for Slice 1 when every lock in
this document is satisfied. Future surfaces become implementation-ready only for
the command subsets whose child plans satisfy the graduation gates below.

This does not claim that every future implementation detail is known. It claims
there are no known open strategy loopholes that would force an implementation
agent to invent product behavior while coding Slice 1 or while starting the
approved subsets of 005-009 after Slice 1.

## Loophole Register

| Loophole | Impact | Fix |
|----------|--------|-----|
| `ContractV1` field floor was under-specified | `doctor`, `context`, and later commands could drift into parallel repo truth | Lock minimum fields and require byte-equal core fixtures. |
| N2 allowed either friendly error or auto-id | Task bootstrap UX and JSON tests could be decided accidentally during coding | Choose structured `task-identity-required`; no auto-id in Slice 1. |
| Extension rules were principle-level only | Downstream repo conventions could leak into Anton core through vague extension use | Require a per-command authority matrix before extension reads. |
| Future surfaces had detailed child plans but unclear start gate | Agents could begin `memory`, `history`, `gates`, `workspace`, or `migrate` too early | Mark 004-009 as blocked until Slice 1 gates and this lock are green. |
| Runnable/write-capable commands were only broadly deferred | A later agent could add `gates run`, `migrate apply`, or `workspace prepare` without safety proof | Require separate security or rollback plans before any execution/write path. |
| `threads` compatibility could be deprecated too early | Existing users could lose the current thin evidence surface before native history parity | Keep `threads` behavior and exit codes stable until native history proves parity. |
| Golden JSON scope was implicit | Reviewers might accept behavior changes without fixture proof | Require named fixtures for every new or changed command contract. |
| Worktree inheritance allowed "inherit or warn" ambiguity | Agents in linked worktrees could still receive the wrong task root silently | Default to inheriting discoverable main-checkout `anton.yaml`; warn only when inheritance cannot be proven. |
| `task` alias was optional during Slice 1 | Implementation could accidentally add a second lifecycle surface while hardening task-state | Defer runtime `task` alias; Slice 1 keeps `task-state` canonical and only locks alias policy. |
| History work-record roots had no schema | Implementers could invent incompatible extension names or scan repo-specific paths | Lock first schema to `extensions.history.work_record_roots` with repo-relative paths only. |
| History sync could duplicate or leak evidence | Repeated syncs could flood receipts or expose raw work/conversation payloads | Require deterministic receipt ids, idempotent sync, bounded summaries, hashes, and redaction fixtures. |
| `migrate plan` was approved before v2 schema existed | Migration preview could invent target config fields while coding | Block `migrate plan` until a v2 config schema lock exists. |

## Hard Decisions

- Slice 1 implementation scope is only `ContractV1`, `doctor/context`, worktree
  config inheritance, N2 task bootstrap error, `task` alias policy without a
  runtime alias, and handoff contract consumption.
- `ContractV1` minimum field floor is:
  - literal `schema_version`
  - adapter name
  - environment
  - execution context
  - resolved config
  - task identity
  - checks or findings
  - summary
  - prompt-contract data
- `doctor --json` and `context --json` must share byte-equal core contract data
  after removing command wrapper fields.
- N2 is a structured friendly error:
  - error code: `task-identity-required`
  - exit code: non-zero command failure
  - no files written
  - message mentions `ANTON_TASK_ID`, `task/<id_slug>` branch, or running inside
    an existing task bundle
  - no deterministic fallback id in Slice 1
- Extensions are inert unless a command explicitly declares an authority rule.
- Future surfaces are implementation-approved only for the exact command subsets
  allowed by their child plans.
- Linked worktrees inherit discoverable main-checkout `anton.yaml` by default;
  warning fallback is allowed only when inheritance cannot be proven.
- `extensions.history.work_record_roots` is the first declared project
  work-record root schema: list of repo-relative paths, advisory only.
- `history sync` must be idempotent over unchanged sources and must not emit full
  raw conversation or work-record payloads by default.
- `migrate plan` is not approved until the target v2 config schema is locked.

## Command Authority Matrix

| Command family | Reads core contract | Reads extensions | Writes state | May execute external tools | Authority level |
|----------------|---------------------|------------------|--------------|----------------------------|-----------------|
| `doctor` | Yes | Opaque/advisory only | No | Existing `codex-threads`/toolchain checks only | Authoritative for repo contract checks |
| `context` | Yes | Opaque/advisory only | No | No required providers | Authoritative projection of `ContractV1` |
| `task-state` | Yes | No in Slice 1 unless separately declared | Yes, only canonical task bundle | No | Authoritative for task lifecycle |
| `task` alias | Blocked in Slice 1 | Blocked in Slice 1 | Blocked in Slice 1 | No | Policy only; later thin alias must delegate to `task-state` |
| `handoff` | Yes | Advisory only | Output pack only | No | Read-only summary of contract and task state |
| `adopt plan` | Yes | Advisory only | No | No provider calls in first slice | Advisory gap report |
| `memory status/update` | Yes | Advisory only | Append-only `.anton/memory/events.jsonl` | No | Advisory, never overrides contract |
| `history show/sync` | Yes | Advisory archive and working-memory metadata | Append-only `.anton/history/receipts.jsonl` | No external binary; bounded local session and repo record reads | Advisory evidence |
| `gates list/check` | Yes | Reads `anton.yaml` gate metadata | No | No | Declarative only |
| `gates run` | Blocked | Blocked until security plan | Append-only run receipts only after plan | Blocked until security plan | Not approved |
| `workspace inspect/check` | Yes | Reads `threads.workspace_roots` as read-only workspace roots | No | No | Inspection only |
| `workspace prepare` | Blocked | Blocked until safety plan | Blocked until rollback/idempotency proof | No | Not approved |
| `migrate plan` | Blocked until v2 schema lock | Reads config versions after schema lock | No | No | v1-to-v2 preview only after schema lock |
| `migrate apply` | Blocked | Blocked until migration plan | Blocked until snapshot/rollback proof | No | Not approved |

## Slice 1 Acceptance Matrix

| Gate | Required proof |
|------|----------------|
| Contract field floor | Unit tests prove `ContractV1` includes every minimum field. |
| Doctor/context parity | Golden fixture proves shared core data is byte-equal across `doctor --json` and `context --json`. |
| N1 worktree inheritance | Test fixture proves linked worktree inherits discoverable main-checkout `anton.yaml` and entrypoint, or warns explicitly when inheritance cannot be proven. |
| N2 task bootstrap | Golden fixture proves missing task identity returns `task-identity-required` and writes no files. |
| Task alias containment | App fixture proves `task` is not registered in Slice 1; later alias requires a separate thin-alias fixture. |
| Extension containment | Tests or code review show no Slice 1 command interprets extension fields without a matrix entry. |
| Backward compatibility | Existing `doctor`, `task-state`, `handoff`, and `threads` golden fixtures either remain stable or change only with explicit fixture diff notes. |
| Future-surface containment | No `memory`, native `history`, runnable `gates`, `workspace prepare`, `entrypoint sync`, or `migrate apply` code lands in Slice 1. |
| Docs alignment | README and plans identify `context --json` or `doctor --json` as the first-run contract surface without contradicting each other. |

## Future Surface Graduation Gates

A future surface may move from roadmap-ready to implementation-ready only when it
has all of the following:

- command-specific authority matrix
- explicit read/write boundary
- golden JSON fixture list
- failure and exit-code policy
- safety plan for any write or external execution
- compatibility policy for existing commands
- documentation update targets

`gates run`, `workspace prepare`, `entrypoint sync`, and `migrate apply` also
require separate security or rollback review before implementation.

## Graduated Future Surfaces

The following child plans satisfy the graduation gates for their approved
command subsets, but still depend on Slice 1 `ContractV1` landing first:

| Plan | Approved after Slice 1 | Still blocked |
|------|------------------------|---------------|
| `005` adopt | `adopt plan` | `adopt apply` |
| `006` memory | `memory status`, `memory update` | authority promotion, pruning |
| `007` history | `history show`, `history sync` with embedded local Codex archive reader and project working-memory reader | extra providers, `threads` deprecation |
| `008` gates | `gates list`, `gates check` | `gates run` |
| `009` maintenance | `entrypoint check`, `workspace inspect/check`; `migrate plan` only after v2 schema lock | `entrypoint sync`, `workspace prepare`, `migrate apply` |

## Review Loop Result

- Loop 1 found strategy loopholes in N2, `ContractV1`, extension authority, and
  future-surface start gates.
- Fixes were applied by locking N2, adding the `ContractV1` field floor, adding
  this authority matrix, and gating future surfaces behind this file.
- Loop 2 found loopholes in worktree inheritance, runtime `task` alias timing,
  history work-record schema, history idempotence/privacy, and premature
  migration preview approval.
- Fixes were applied by locking worktree inheritance behavior, deferring the
  runtime `task` alias, naming `extensions.history.work_record_roots`, requiring
  idempotent/redacted history sync, and blocking `migrate plan` until v2 schema
  lock.
- Loop 3 re-scanned the plan family for remaining strategy-level ambiguity
  markers. Remaining deferred items are limited to non-core field names, human
  wording, package names, pruning, later runner security, and later mutating
  command safety; none are approved implementation behavior that would require a
  Slice 1 or graduated-surface implementer to invent product strategy.
- A plan can be called confidence-locked when the verification commands find no
  remaining ambiguous strategy markers and reviewers can trace every Slice 1
  behavior to a gate above.

## Sources & References

- Master roadmap: `docs/plans/2026-05-08-001-feat-anton-vnext-master-roadmap-plan.md`
- Contract/context slice: `docs/plans/2026-05-08-002-feat-anton-contract-context-slice-plan.md`
- Task/handoff slice: `docs/plans/2026-05-08-003-feat-anton-task-handoff-slice-plan.md`
- Future surfaces roadmap: `docs/plans/2026-05-08-004-feat-anton-future-surfaces-roadmap-plan.md`
- Current doctor contract source: `internal/doctor/doctor.go`
- Current task identity inference: `internal/adapter/default.go`
- Current task identity model: `internal/adapter/task_identity.go`
