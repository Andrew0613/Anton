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

## Relationships

- A **Config Contract** contributes declared settings to one **Execution Contract**
- An **Execution Context** is one input family inside an **Execution Contract**
- The **Context Command** presents the same **Execution Contract** that `doctor` uses
- The **Doctor Command** may include health data beyond the **Execution Contract**
- An **Execution Contract** may be rendered as one **Prompt Contract**
- A **Prompt Contract** must not resolve repository state independently of the **Execution Contract**

## Example dialogue

> **Dev:** "Should `anton context` read `anton.yaml` itself?"
> **Domain expert:** "No. It should ask the shared builder for the **Execution Contract**. The **Config Contract** is only one input to that builder."

## Flagged ambiguities

- "contract" was used for `anton.yaml`, the runtime `ContractV1` payload, and prompt text. Resolved: use **Config Contract**, **Execution Contract**, and **Prompt Contract**.
- "`context` versus `doctor`" was ambiguous as a first-run path. Resolved: **Context Command** is the preferred first-run briefing; **Doctor Command** is the health and remediation surface.
