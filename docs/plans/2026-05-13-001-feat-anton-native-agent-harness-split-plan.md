---
title: feat: Anton-native agent harness implementation split
type: feat
status: draft
date: 2026-05-13
origin: /home/puyuandong/.gstack/projects/Andrew0613-Anton/puyuandong-feat-anton-vnext-upgrade-design-20260513-061704.md
issues: [5, 6, 7, 8, 9]
---

# feat: Anton-native agent harness implementation split

## Overview

Anton vNext should be split as one agent-harness loop, not five unrelated
feature branches.

The direct user is a coding agent. The repo owner is an indirect user. That
means command contracts, JSON shapes, exit codes, actionable remediation, and
safe no-write behavior are product surfaces, not implementation details.

The target loop is:

```text
context -> preflight -> task-state -> workspace/gates -> handoff
enter      check        record        keep clean          transfer
```

The open issues map to this loop:

| Issue | Product surface | Role in loop |
| --- | --- | --- |
| #6 | `task-state` topic-layer and status lifecycle | Authoritative task state |
| #8 | `preflight` investigation/implementation profiles | Safe start |
| #9 | `workspace refs` and `migrate readiness` | Repo cleanliness before path/move work |
| #7 | source-aware `handoff build` and result persistence | Transfer to next agent |
| #5 | runnable gates | Validation, split into declarative now and execution later |

## Strategic Decision

Choose Approach B from the design doc: Anton-native harness architecture.

Do not copy agent-pack's product center. agent-pack is a durable work packet and
brief system. Anton is the repo harness contract a coding agent calls to work
safely inside an existing repository.

## Premises

1. `ContractV1` remains the shared source of repo truth for command projections.
2. `task-state` remains the authoritative lifecycle state surface.
3. Repo-specific layouts are declared in `anton.yaml`, never hard-coded by repo
   name.
4. Status words are shared across surfaces: `ok`, `degraded`, `warning`,
   `blocked`, and `skipped`.
5. Read/check/plan commands land before write/run/apply commands.
6. Runnable gates require a separate command-execution safety plan.
7. PhysEdit is the first demanding adopter, not a special runtime mode.

## What Already Exists

| Existing code | Reuse |
| --- | --- |
| `internal/contract/contract.go` | `ContractV1`, `Check`, `Finding`, `Summary` field floor |
| `internal/adapter/config.go` | strict `anton.yaml` v1 parsing, gates metadata, history extension schema |
| `internal/adapter/default.go` | task identity inference, canonical task bundle layout, status reads/writes |
| `internal/contextcmd/context.go` | current `context` projection; should become a pure contract/readiness entry command |
| `internal/doctor/doctor.go` | existing health probes, remediation, and current contract collection |
| `internal/taskstate/taskstate.go` | lifecycle commands and task bundle file validation |
| `internal/handoff/handoff.go` | first contract-aware handoff pack |
| `internal/gates/*` | declarative gate metadata and blocked `gates run` stub |
| `internal/workspace/workspace.go` | read-only workspace root inspect/check with path-boundary checks |
| `internal/migrate/migrate.go` | blocked v2 migration preview placeholder |
| `internal/history/*` | append-only receipts, local archive/work-memory ingestion, redaction hooks |
| `internal/memory/*` | advisory memory status/update with freshness and conflict warnings |
| `internal/app/app.go` | command registration and global `--json` normalization |

## NOT In Scope

- No daemon, scheduler, agent runner, Kanban board, queue, budget system, or UI.
- No agent-pack-compatible pack manifest system.
- No `if repo == physedit` branches in Anton core.
- No `gates run` execution in the same PR as declarative gate improvements.
- No `migrate apply`, `workspace prepare`, or `entrypoint sync`.
- No broad history-provider ecosystem beyond bounded local sources already
  planned.

## Shared Objects

Before implementing #6-#9, define the shared objects that each command reads or
writes. This prevents five surfaces from inventing incompatible JSON contracts.

### Harness Status

Use the same status vocabulary everywhere:

| Status | Meaning | Exit behavior |
| --- | --- | --- |
| `ok` | Surface is usable and no blocking findings exist | exit 0 |
| `degraded` | Usable with meaningful risks or missing optional capability | exit 1 for health commands, exit 0 only for pure reports that document degradation |
| `warning` | Non-blocking warning in a sub-check or finding | parent decides summary |
| `blocked` | Agent should not proceed without fixing or asking | exit 1 |
| `skipped` | Check intentionally not run because optional or out of profile | exit 0 unless all required checks skipped |

### Harness Finding

Common finding shape:

```go
type HarnessFinding struct {
    Level       string `json:"level"`
    Code        string `json:"code"`
    Surface     string `json:"surface"`
    Path        string `json:"path,omitempty"`
    Message     string `json:"message"`
    Remediation string `json:"remediation,omitempty"`
}
```

This can live in `internal/contract` or a new small package if import cycles
appear. Prefer reuse over abstraction if only two packages need it.

### Harness Receipt

Common receipt shape for append-only state changes:

```go
type HarnessReceipt struct {
    SchemaVersion string            `json:"schema_version"`
    ID            string            `json:"id"`
    Type          string            `json:"type"`
    Source        string            `json:"source"`
    Status        string            `json:"status"`
    RecordedAt    string            `json:"recorded_at"`
    Summary       string            `json:"summary"`
    Artifacts     []string          `json:"artifacts,omitempty"`
    Payload       map[string]string `json:"payload,omitempty"`
}
```

Reuse existing history and task evidence shapes where possible. Do not migrate
existing receipt files in this plan.

### Repo Cleanliness Finding

Workspace, migrate readiness, handoff, and preflight all need to describe dirty
repo risks.

First shape:

```go
type CleanlinessFinding struct {
    Code        string `json:"code"`
    Level       string `json:"level"`
    Path        string `json:"path,omitempty"`
    Message     string `json:"message"`
    Remediation string `json:"remediation,omitempty"`
}
```

Do not add a git porcelain parser abstraction until at least two commands need
the same parsed data.

## Architecture Diagram

```text
                +------------------------+
                | internal/adapter       |
                | context + config       |
                +-----------+------------+
                            |
                            v
                +------------------------+
                | internal/contract      |
                | ContractV1 + findings  |
                +-----------+------------+
                            |
        +-------------------+-------------------+
        |                   |                   |
        v                   v                   v
  context/doctor       task-state          preflight
  read contract        write state         read contract + probes
        |                   |                   |
        +---------+---------+---------+---------+
                  |                   |
                  v                   v
              workspace/gates      handoff
              read/check/plan      read state + contract
```

## Implementation Units

### Unit 0: Shared Contract Addendum

**Goal:** Add a small architecture addendum that names shared statuses,
findings, receipts, and command authority before code changes begin.

**Issues:** all

**Files:**
- Modify: `docs/plans/2026-05-13-001-feat-anton-native-agent-harness-split-plan.md`
- Optional modify: `docs/plans/2026-05-08-010-feat-anton-vnext-confidence-lock-plan.md`

**Approach:**
- Keep the current confidence lock intact.
- Add this split plan as the working implementation index for #5-#9.
- Extract the idea of a pure contract resolver from `doctor`: `context` should
  read repo/config/task identity facts without writability probes, Go toolchain
  checks, or optional integration checks.
- Keep probes in `doctor` and `preflight`, where cost and side effects are
  expected by the command name.
- Do not add new code in this unit.

**Acceptance:**
- A later agent can identify the owner package, status vocabulary, and blocked
  write/execute surfaces without rereading the office-hours artifact.
- `context` is specified as the cheap first command in the agent loop, while
  `doctor` and `preflight` own health/probe behavior.

### Unit 1: Task-State Layout Extensions (#6)

**Goal:** Make `task-state` support declared topic-layer task layouts and
status schemas without hard-coding PhysEdit.

**Files:**
- Modify: `internal/adapter/config.go`
- Modify: `internal/adapter/default.go`
- Modify: `internal/adapter/status_yaml.go`
- Modify: `internal/taskstate/taskstate.go`
- Modify: `internal/taskstate/taskstate_test.go`
- Add fixtures under: `internal/taskstate/testdata/`
- Update: `README.md`

**Config shape:**

```yaml
tasks:
  root: project_progress
  layout: topic-layer
  status_schema: physedit-v1
  card_sync: true
```

If the exact names feel too specific during implementation, keep the meaning and
choose clearer names. Do not use repo names as enum values except for a
documented schema compatibility label.

**Commands:**
- `anton task-state check --schema anton|auto|physedit-v1 --json`
- `anton task-state env --machine-type ... --proxy ... --cwd ... --json`
- `anton task-state service add --name ... --kind ... --status ... --reopen-hint ... --json`
- `anton task-state freshness --canonical-truth ... --checked-at ... --json`
- `anton task-state sync-card --json`

**Non-goals:**
- No migration of existing task bundles.
- No rewriting prose task cards unless `sync-card` is explicitly called.
- No daemon or service process management.

**Tests:**
- Anton-native `.anton/tasks/active/<id>/status.yaml` still passes.
- Topic-layer bundle path resolves from `ANTON_TASK_ID`.
- Topic-layer bundle path resolves from current working directory inside bundle.
- Existing schema fields are preserved after `env`, `service add`, and
  `freshness`.
- Unknown schema returns a structured config error.
- `sync-card` dry or minimal update fixture keeps prose outside the generated
  freshness block untouched.

**Acceptance:**
- The agent can use Anton instead of direct YAML edits for lifecycle metadata.
- PhysEdit-like layouts are declared through config, not code branches.

### Unit 2: Preflight Profiles (#8)

**Goal:** Add a read-only start-work command that tells the coding agent whether
it can begin an investigation or implementation lane.

**Files:**
- Add: `internal/preflight/preflight.go`
- Add: `internal/preflight/preflight_test.go`
- Modify: `internal/app/app.go`
- Modify: `README.md`

**Commands:**
- `anton preflight --profile investigation --json`
- `anton preflight --profile implementation --json`

**Initial checks:**
- Pure contract/context resolution without health probes.
- Config validity and entrypoint existence.
- Task identity and task-state readability.
- Working directory writable probe with cleanup.
- Git repo/worktree state summary.
- Gate declaration health via `internal/gates`.
- Optional integration signals as `skipped` unless configured.

**Profile differences:**
- `investigation`: missing task identity is `warning` unless repo policy says
  otherwise.
- `implementation`: missing task identity is `blocked`.

**Tests:**
- Clean repo with task identity returns `ok`.
- Missing optional integration returns `skipped`, not failure.
- Missing task identity differs between profiles.
- Writable probe cleans up after itself.
- Invalid config returns structured `blocked`.

**Acceptance:**
- A coding agent can run one command before work and know whether to proceed,
  fix local state, or ask the user.

### Unit 3: Workspace Refs and Migration Readiness (#9)

**Goal:** Extend workspace/migration read-only checks so agents can inspect path
references and move readiness before touching files.

**Files:**
- Modify: `internal/workspace/workspace.go`
- Modify: `internal/workspace/workspace_test.go`
- Modify: `internal/migrate/migrate.go`
- Modify: `internal/migrate/migrate_test.go`
- Optional add: `internal/workspace/refs.go`
- Optional add: `internal/migrate/readiness.go`
- Update: `README.md`

**Commands:**
- `anton workspace refs --target <path> --json`
- `anton migrate readiness --target <path> --json`

**Output must include:**
- Target path, normalized path, and repo-boundary status.
- Reference hits across docs, scripts, config, and task bundles.
- Active worktree/branch occupancy where detectable from git metadata.
- Task bundle status if target overlaps task roots.
- CI/checker surfaces likely affected.
- Summary with blockers, warnings, and a go/no-go recommendation.

**Safety:**
- Read-only.
- Target is required. Anton must not invent migration intent.
- No filesystem moves.
- No `migrate apply`.

**Tests:**
- Target outside repo is blocked.
- Symlink escape is blocked.
- References in Markdown, YAML, Go, and shell files are found.
- Ignored/generated directories are either excluded or clearly marked as
  skipped.
- Worktree occupancy fixture reports branch/path conflicts.

**Acceptance:**
- Agents can replace ad hoc path inventory/readiness scripts with Anton
  read-only reports.

### Unit 4: Source-Aware Handoff and Result Persistence (#7)

**Goal:** Expand `handoff build` into a compact, source-aware packet that a
fresh coding agent can use without rereading full history.

**Files:**
- Modify: `internal/handoff/handoff.go`
- Modify: `internal/handoff/handoff_test.go`
- Reuse: `internal/history/redact.go`
- Reuse: `internal/history/archive.go`
- Optional add: `internal/handoff/sources.go`
- Update: `README.md`

**Commands:**
- `anton handoff build --source manual|codex|claude --session-id <id> --json`
- `anton handoff persist-results --worktree-root <path> --run-dir <path> --dry-run --json`

**Output must include:**
- Contract summary.
- Task status summary.
- Git branch, short SHA, dirty tracked files, untracked files.
- Validation receipts already present.
- Blockers and user decisions.
- Exact next commands.
- Redacted source snippets only when source/session is explicit.

**Safety:**
- `build` is read-only.
- `persist-results` defaults to dry-run.
- No large artifact copy without explicit apply flag in a later plan.
- Redact obvious secrets before including extracted source text.

**Tests:**
- Manual handoff works without history.
- Codex source with fake session extracts bounded summary.
- Secret-like tokens are redacted.
- Dirty/untracked files appear in JSON.
- `persist-results --dry-run` reports copy plan and writes nothing.

**Acceptance:**
- A fresh agent can resume from handoff output with exact next commands and
  clear blockers.

### Unit 5: Gate Contract Split (#5)

**Goal:** Split #5 into a safe declarative gate improvement now and runnable
gate execution later.

**Part A: Declarative gate contract now**

**Files:**
- Modify: `internal/gates/gates.go`
- Modify: `internal/gates/command.go`
- Modify: `internal/gates/gates_test.go`
- Update: `README.md`

**Work:**
- Improve required/advisory summaries.
- Add profile concepts such as `required_for: [implementation, handoff, closeout]`
  only if the naming is locked in config docs.
- Add remediation hints.
- Make `command` optional inert metadata until the runner safety plan lands.
- Ensure shell-like command metadata is visible as a warning but never required
  for declarative gate validity.
- Keep `gates run` blocked.

**Part B: Runnable gates later**

Create a separate safety plan before implementation:

- argv arrays only;
- no shell strings;
- command allowlist or repo-declared trust boundary;
- timeout;
- cwd boundary;
- output byte limits;
- append-only run receipts;
- no destructive gates by default.

**Tests for Part A:**
- Required/advisory gates summarize correctly.
- Shell-like metadata is refused or warned but never executed.
- Destructive gates are visible and inert.
- `gates run --json` returns the existing not-approved error.

**Acceptance:**
- Agent can inspect gate expectations now.
- No command execution lands without safety review.

### Unit 6: README and Agent-Facing Loop

**Goal:** Make the product promise and command loop obvious to an external user
and to a coding agent.

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md` when code lands

**README additions:**

```text
Anton is a repo harness CLI for coding agents.

Start loop:
  anton context --json
  anton preflight --profile implementation --json
  anton task-state check --json
  anton gates check --json
  anton handoff build --json
```

**Acceptance:**
- The README distinguishes Anton from agent-pack without naming it defensively.
- The first five minutes are copy-pasteable.

## Test Diagram

```text
Shared contract
  -> pure context resolver tests
  -> doctor/context parity tests where both intentionally overlap
  -> preflight consumes contract
  -> handoff includes contract

Task-state extension
  -> anton-native schema fixture
  -> topic-layer schema fixture
  -> env/service/freshness mutation fixtures
  -> sync-card prose-preservation fixture

Preflight
  -> investigation profile
  -> implementation profile
  -> optional checks skipped
  -> writable probe cleanup

Workspace/migrate readiness
  -> target normalization
  -> reference hit scan
  -> symlink/path escape
  -> worktree occupancy
  -> task-root overlap

Handoff
  -> manual source
  -> codex/claude fake source
  -> dirty git state
  -> redaction
  -> persist dry-run writes nothing

Gates
  -> declarative metadata
  -> unsafe command metadata warning
  -> destructive gate inert
  -> gates run remains blocked
```

## Error & Rescue Registry

| Error | Surface | Agent sees | Rescue |
| --- | --- | --- | --- |
| missing task identity | `task-state`, `preflight` | `task-identity-required` / `blocked` | set `ANTON_TASK_ID`, switch to task branch, or enter bundle |
| unsupported task layout | `task-state` | config error | fix `anton.yaml` layout/schema |
| optional integration missing | `preflight` | `skipped` | continue or configure integration |
| target path escapes repo | `workspace refs`, `migrate readiness` | `blocked` | provide repo-local target |
| symlink escape | `workspace`, `history`, `handoff` | `blocked` or fatal warning | replace symlink with regular repo-local path |
| gate has shell-like command | `gates check` | warning/inert metadata | use argv array and wait for runnable gate safety plan |
| source history includes secret | `handoff build` | redacted snippet | inspect source manually if redaction removes needed context |
| dirty files present at handoff | `handoff build` | warning with file list | commit, stash, or mention intentionally dirty files |

## Failure Modes Registry

| Failure mode | Severity | Prevention |
| --- | --- | --- |
| PhysEdit layout leaks into Anton core | High | require config-declared layout and schema enum, no repo-name branches |
| Five issues create five status vocabularies | High | shared harness status table before code |
| `context` becomes a slow health command | High | pure contract resolver first; probes stay in `doctor`/`preflight` |
| `preflight` becomes slow mandatory startup tax | Medium | profiles and optional checks default to skipped |
| `workspace refs` misses generated or ignored references | Medium | report skipped roots and configurable scan boundaries |
| `handoff` grows too large for fresh agents | Medium | bounded summaries and exact next commands |
| Runnable gates execute unsafe shell content | Critical | split into later safety plan, argv-only, timeout, cwd boundary |
| `migrate readiness` becomes de facto `migrate apply` | High | read-only and target-required |

## Developer Journey Map

| Stage | Command | Expected agent outcome |
| --- | --- | --- |
| Enter repo | `anton context --json` | Know repo root, entrypoint, task root, contract warnings |
| Start work | `anton preflight --profile implementation --json` | Know whether work is safe or blocked |
| Inspect task | `anton task-state check --json` | Know lifecycle, schema, missing files |
| Update progress | `anton task-state pulse/env/service/freshness --json` | Update state without manual YAML editing |
| Check boundaries | `anton workspace check --json` | Know workspace roots and path risks |
| Plan move | `anton workspace refs --target ... --json` | Know path references before moving |
| Validate expectations | `anton gates check --json` | Know declared required/advisory gates |
| Transfer work | `anton handoff build --json` | Give next agent compact state and exact next commands |
| Preserve results | `anton handoff persist-results --dry-run --json` | See safe result persistence plan |

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|-------|----------|----------------|-----------|-----------|----------|
| 1 | CEO | Choose Anton-native harness architecture | Mechanical | Explicit over clever | Matches user-stated coding-agent target and repo cleanliness goal | agent-pack clone |
| 2 | CEO | Keep all five surfaces but share one contract/state model | Mechanical | Choose completeness | User explicitly said all five are important; shared model avoids command sprawl | single-feature MVP |
| 3 | Eng | Implement task-state layout extension before preflight/handoff | Mechanical | DRY | Other surfaces need authoritative task root/schema truth | preflight first |
| 4 | Eng | Split runnable gates from declarative gate contract | Mechanical | Safety | Command execution is a separate trust boundary | implement runnable gates now |
| 5 | DX | Optimize command loop for coding agents and JSON first | Mechanical | User target clarity | Direct user is the agent, human prose is secondary | human-first brief |
| 6 | Eng | Keep `context` pure and put probes in `doctor`/`preflight` | Mechanical | Fast first command | Coding agents need a cheap repo contract read before expensive checks | context as doctor alias |

## Review Summary

### CEO

The problem is framed correctly: Anton is a repo harness, not a task packet.
The major strategic risk is allowing real PhysEdit pressure to collapse the
repo-agnostic boundary. This split plan keeps PhysEdit as the first demanding
adopter while forcing layout and schema through `anton.yaml`.

### Engineering

The key architecture requirement is reuse of existing `internal/adapter`,
`internal/contract`, `internal/taskstate`, `internal/gates`, `internal/workspace`,
and `internal/handoff` surfaces. The main hidden complexity is schema extension:
topic-layer task state must not become a second adapter unless the existing
default adapter is no longer readable.

### DX

The developer experience should be agent-first:

```bash
anton context --json
anton preflight --profile implementation --json
anton task-state check --json
anton gates check --json
anton handoff build --json
```

Every failure must say what happened, why it matters, and what exact command or
config change unblocks the agent.

### Design

Skipped. This plan has no UI/product screen scope. Text mentions of "layout" are
schema/layout references, not frontend work.

## Cross-Phase Themes

- Shared status taxonomy is the first contract to lock.
- Task-state schema extension is the first implementation proof.
- Write/execute surfaces must trail read/check/plan surfaces.
- README must teach the command loop, not just list commands.

## Implementation Control Addendum

This plan is the working implementation index for issues #5-#9. Treat the five
surfaces as one agent-harness loop, with `internal/adapter` and
`internal/contract` holding shared repo truth and command packages projecting
that truth for specific agent decisions.

Package authority:

| Package | Authority |
| --- | --- |
| `internal/adapter` | Config parsing, task-root/layout resolution, task identity, status read/write compatibility |
| `internal/contract` | Shared contract objects, status/finding vocabulary, prompt contract rendering |
| `internal/contextcmd` | Cheap first command: repo/config/task identity facts only |
| `internal/doctor` | Health probes, remediation, optional integration checks |
| `internal/preflight` | Start-work readiness by profile, using contract facts plus bounded probes |
| `internal/taskstate` | Lifecycle metadata mutations and task bundle validation |
| `internal/workspace` and `internal/migrate` | Read-only path/reference/readiness reports |
| `internal/gates` | Declarative gate metadata only; command execution stays blocked |
| `internal/handoff` | Compact transfer packet and dry-run result persistence planning |

`context` must stay a pure, cheap contract read. It should not perform writable
probes, Go toolchain checks, optional integration checks, reference scans, or
history extraction. Those belong in `doctor`, `preflight`, `workspace refs`, or
`handoff build` according to user intent.

Blocked write/execute surfaces in this plan:

- `gates run` remains unavailable.
- `migrate apply` remains unavailable.
- `workspace prepare` remains unavailable.
- `handoff persist-results` is dry-run only.
- `sync-card` may update only its generated freshness block and must preserve
  surrounding prose.

## Verification Commands

Use the repo's supported Go toolchain. On this machine that has previously been:

```bash
~/.local/share/go1.22.0/bin/go test ./...
```

Also run:

```bash
git diff --check
anton context --json
anton doctor --json
```

After each unit lands, add focused tests before broadening scope.

## Approval Gate

Recommended approval: proceed with this split.

Taste choices surfaced:

1. Config names for task layout/schema.
   - Recommended: `tasks.layout` and `tasks.status_schema`.
   - Alternative: put these under `extensions.task_state.*`.

2. Whether `preflight` should exit 1 on `degraded`.
   - Recommended: health/start-work commands exit 1 for degraded so agents pay
     attention.
   - Alternative: exit 0 for degraded and rely on JSON status.

3. Whether `handoff persist-results` belongs under `handoff` or `workspace`.
   - Recommended: keep under `handoff` because the user intent is transfer and
     persistence.
   - Alternative: put under `workspace` because it manages filesystem movement.
