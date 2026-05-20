# Anton Harness Context

Anton is a reusable CLI-first harness for coding-agent repository workflows. This context defines Anton-specific language so plans, code, and handoffs do not use the same word for different contract surfaces.

## Language

**Config Contract**:
The static repo-declared Anton configuration, normally loaded from `anton.yaml`.
_Avoid_: Canonical repo contract, config truth

**Execution Contract**:
The resolved runtime view Anton gives an agent after combining the current directory, repository state, worktree state, config, entrypoint, and task identity.
_Avoid_: Context contract, doctor contract

**Execution Context**:
The current environment facts Anton resolves for a command, including working directory, repo/worktree shape, branch, configured paths, and task identity.
_Avoid_: Memory, background context, documentation context

**Context Command**:
The `anton context` command surface that presents the **Execution Contract** as the preferred first-run agent briefing.
_Avoid_: Context resolver, independent context source

**Doctor Command**:
The `anton doctor` command surface that wraps the **Execution Contract** with harness health checks, warnings, and remediation.
_Avoid_: First-run briefing, contract owner

**Prompt Contract**:
A human-readable projection of the **Execution Contract** for agent prompts and handoffs.
_Avoid_: Contract source of truth, resolver

**Run Manifest**:
A passive, task-scoped execution record for checklist items, attempts,
validation receipts, audit notes, and closure state. The run manifest describes
work done by an external agent or human; it does not launch or supervise that
work.
_Avoid_: Agent runner backend, scheduler state, daemon state

**Planning File Projection**:
Markdown files such as `task_plan.md`, `findings.md`, and `progress.md` when
they are generated for compatibility, review, or adopter migration. They may
mirror Anton state, but they are not the preferred canonical model for new
adopters.
_Avoid_: Mandatory planning-with-files contract, source of truth

**Gates Runner**:
A bounded command surface for running explicitly declared argv-style validation
gates and recording receipts. It is not shell automation, a queue, or a
scheduler.
_Avoid_: Shell wrapper, CI backend, agent runner

## Relationships

- A **Config Contract** contributes declared settings to one **Execution Contract**
- An **Execution Context** is one input family inside an **Execution Contract**
- The **Context Command** presents the same **Execution Contract** that `doctor` uses
- The **Doctor Command** may include health data beyond the **Execution Contract**
- An **Execution Contract** may be rendered as one **Prompt Contract**
- A **Prompt Contract** must not resolve repository state independently of the **Execution Contract**
- A **Run Manifest** is tied to task identity and complements **Task State**; it
  must not duplicate task lifecycle ownership
- A **Planning File Projection** may be emitted from Anton state for compatibility
  but must not become a separate resolver
- A **Gates Runner** may append validation receipts to a **Run Manifest** only
  when the caller explicitly invokes it

## Example dialogue

> **Dev:** "Should `anton context` read `anton.yaml` itself?"
> **Domain expert:** "No. It should ask the shared builder for the **Execution Contract**. The **Config Contract** is only one input to that builder."

## Flagged ambiguities

- "contract" was used for `anton.yaml`, the runtime `ContractV1` payload, and prompt text. Resolved: use **Config Contract**, **Execution Contract**, and **Prompt Contract**.
- "`context` versus `doctor`" was ambiguous as a first-run path. Resolved: **Context Command** is the preferred first-run briefing; **Doctor Command** is the health and remediation surface.
- "runner" was ambiguous because Anton borrows run-manifest ideas from
  agent-runner. Resolved: Anton records passive run state and receipts; it does
  not run Codex, Claude, or any other coding-agent backend.
- "planning files" was ambiguous because old adopters used them as hard harness
  rules. Resolved: planning files are compatibility or projection artifacts for
  migration, while new adopters should use Anton-native state.
