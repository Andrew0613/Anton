#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-$HOME/.local/share/go1.22.0/bin/go}"
WORK="${TMPDIR:-/var/tmp}/anton-dogfood-harness-consolidation-$$"
BIN="$WORK/anton"

cleanup() {
  rm -rf "$WORK"
}
trap cleanup EXIT

mkdir -p "$WORK"
"$GO_BIN" build -o "$BIN" "$ROOT/cmd/anton"

run_manifest_repo="$WORK/run-manifest-only"
mkdir -p "$run_manifest_repo/.git" "$run_manifest_repo/.anton/tasks/active/demo_task"
printf 'ref: refs/heads/main\n' > "$run_manifest_repo/.git/HEAD"
cat > "$run_manifest_repo/AGENTS.md" <<'EOF'
# Test Agent

See README.md.
EOF
cat > "$run_manifest_repo/README.md" <<'EOF'
# Run Manifest Dogfood

AGENTS.md is the entrypoint. anton.yaml declares the Anton contract.
EOF
cat > "$run_manifest_repo/anton.yaml" <<'EOF'
version: 1
entrypoint:
  path: AGENTS.md
tasks:
  root: .anton/tasks
  planning_mode: run_manifest
run:
  enabled: true
  manifest: run.json
  receipts_dir: receipts
threads:
  default_project_strategy: repo-root
gates:
  - name: smoke
    type: command
    required_for: [handoff]
    command:
      argv: ["true"]
gate_profiles:
  handoff:
    required: [smoke]
EOF

(
  cd "$run_manifest_repo"
  export ANTON_TASK_ID=demo_task
  "$BIN" context --json >/dev/null
  "$BIN" task-state init --json >/dev/null
  "$BIN" preflight --profile implementation --json >/dev/null
  "$BIN" run init --json >/dev/null
  "$BIN" run task add --id u1 --title "Dogfood run manifest" --json >/dev/null
  "$BIN" run task set --id u1 --status done --note "dogfood complete" --json >/dev/null
  "$BIN" gates check --json >/dev/null
  "$BIN" gates run --profile handoff --dry-run --attach-run --json >/dev/null
  "$BIN" handoff build --json >/dev/null
)

heavy_repo="$WORK/heavy-harness"
mkdir -p "$heavy_repo/.git" "$heavy_repo/scripts" "$heavy_repo/docs"
printf 'ref: refs/heads/main\n' > "$heavy_repo/.git/HEAD"
cat > "$heavy_repo/AGENTS.md" <<'EOF'
# Test Agent

Use planning-with-files and update task_plan.md, findings.md, and progress.md.
See README.md.
EOF
cat > "$heavy_repo/README.md" <<'EOF'
# Heavy Harness Dogfood

AGENTS.md is the entrypoint. anton.yaml declares the Anton contract.
EOF
cat > "$heavy_repo/anton.yaml" <<'EOF'
version: 1
entrypoint:
  path: AGENTS.md
tasks:
  root: .anton/tasks
threads:
  default_project_strategy: repo-root
EOF
cat > "$heavy_repo/scripts/task_status.py" <<'EOF'
print("status.yaml task-state active_task")
EOF
cat > "$heavy_repo/scripts/check_workflow_contracts.py" <<'EOF'
print("workflow contract gate validation receipt")
EOF
cat > "$heavy_repo/docs/research_policy.md" <<'EOF'
GPU cluster dataset benchmark model policy stays project-local.
EOF
(
  cd "$heavy_repo"
  "$BIN" adopt harness-inventory --json >/dev/null
)

echo "dogfood harness consolidation ok"
