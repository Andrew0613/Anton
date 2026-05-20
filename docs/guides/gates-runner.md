# Gates Runner Guide

`gates list` and `gates check` read declarative gate metadata. `gates run` adds
a narrow execution subset for validation commands that are declared in
`anton.yaml`.

This is not a shell wrapper, backend runner, CI service, daemon, scheduler,
queue, or UI. It exists to produce bounded validation receipts that can be
attached to a run manifest and handoff.

## Safety Model

`anton gates run` only executes gates that satisfy all of these rules:

- commands are argv arrays, not shell strings
- cwd stays inside the repo root
- timeout is explicit or defaulted by Anton policy
- stdout and stderr are capped
- exit code and duration are recorded
- destructive gates are blocked by default
- missing or unsafe gate declarations fail before execution
- receipts are append-only artifacts

No command should be routed through `sh -c`, `bash -c`, or equivalent shell
evaluation.

## Example Config

```yaml
gates:
  - name: unit
    type: command
    command:
      argv: ["go", "test", "./..."]
      working_directory: "."
    timeout:
      seconds: 120
  - name: docs
    type: command
    command:
      argv: ["rg", "-n", "run manifest|planning-with-files", "README.md", "docs/guides"]
      working_directory: "."
    timeout:
      seconds: 30
gate_profiles:
  handoff:
    required: ["unit", "docs"]
```

Profiles group declared gates. They do not make Anton a scheduler; a user or
agent must explicitly invoke the command.

## Intended Commands

```bash
anton gates run --gate unit --json
anton gates run --profile handoff --json
anton gates run --profile handoff --attach-run --json
anton gates run --dry-run --json
```

`--dry-run` should show what would run and why. `--attach-run` should append
receipt references to the run manifest only after the caller asks for that
connection.

## Receipts

A gate receipt should include:

- gate name
- argv
- resolved cwd
- start and end time
- duration
- exit code
- status
- stdout and stderr summaries
- truncation flags
- receipt path

Receipts should be safe to include in handoff output. They should describe
validation that already happened; they should not hide background work.

## Project-Specific Gates

Project-specific validation belongs in the adopter repo. Anton can run a
declared command and record its receipt, but the command's domain policy,
thresholds, credentials, and destructive behavior remain the repo's
responsibility.
