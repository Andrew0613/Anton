# Run Manifest Guide

The run manifest is Anton's passive execution record. It captures what happened
during a task without becoming the system that runs the agent.

The design borrows from agent-runner's manifest, lifecycle, and audit concepts,
then narrows them to Anton's boundary: explicit CLI mutations, no daemon, no
scheduler, no queue, no UI, and no coding-agent backend integration.

## Purpose

Use a run manifest to record:

- checklist items for the current task
- attempts and summaries
- validation receipts
- audit notes
- closure state

Task lifecycle still belongs to `anton task-state`. The run manifest should not
invent task identity, close or reopen tasks, or replace status ownership.

For typed-state repos, task truth and inventory are resolved by:

- `anton task resolve --json`
- `anton task list --state active --json`

## Shape

A task-scoped manifest is expected to live beside the task bundle, for example:

```text
.anton/tasks/active/example/run.json
.anton/tasks/active/example/receipts/
```

Representative schema:

```json
{
  "schema_version": 1,
  "task_id": "example",
  "mode": "sidecar",
  "planning_mode": "run_manifest",
  "checklist": [
    {
      "id": "u1",
      "title": "Document passive run manifest support",
      "status": "done",
      "notes": []
    }
  ],
  "attempts": [],
  "audit": []
}
```

`mode` is `sidecar` for this slice. Status values should stay small:
`pending`, `in_progress`, `blocked`, `done`, and `dropped`.

## Commands

```bash
anton run init --json
anton run status --json
anton run task list --json
anton run task add --id u1 --title "Document migration" --json
anton run task set --id u1 --status done --note "Guide added" --json
anton run audit add --kind gate --name docs --status passed --summary "grep ok" --json
anton run close --status done --summary "Ready for handoff" --json
```

Every mutation should be explicit, task-scoped, JSON-serializable, and safe to
include in a handoff. Missing task identity should fail with a structured error
instead of creating anonymous state.

## Planning Files

For existing adopters, `task_plan.md`, `findings.md`, and `progress.md` may stay
as compatibility projections. They are useful when a repo or reviewer still
expects markdown files. New adopters should treat the run manifest plus
`task-state` as the canonical machine-checkable path.

Recommended modes:

- `planning_files`: legacy compatibility
- `hybrid`: migration period with both manifest and planning files
- `run_manifest`: preferred mode for new adopters using the run surface

## Handoff

Handoff should read the run manifest when present and include:

- checklist summary
- latest attempts
- audit notes
- gate receipt paths and outcomes

If a repo only has legacy planning files, handoff should still work and report
that no run manifest was present.
