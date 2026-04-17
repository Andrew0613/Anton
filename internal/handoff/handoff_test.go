package handoff

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandoffBuildJSON(t *testing.T) {
	repoRoot := makeHandoffTempRepoRoot(t)
	writeHandoffFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeHandoffFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	bundleRoot := filepath.Join(repoRoot, ".anton", "state", "active", "demo_task")
	writeHandoffFile(t, filepath.Join(bundleRoot, "task_plan.md"), "# Task Plan\n\n## Goal\nShip stable handoff pack output.\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "findings.md"), "# Findings\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "progress.md"), "# Progress\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "status.yaml"), ""+
		"version: 1\n"+
		"stable:\n  task_id: demo_task\n  created_at: 2026-04-17T00:00:00Z\n"+
		"state:\n  lifecycle: review\n  updated_at: 2026-04-17T00:10:00Z\n"+
		"machine:\n  host: test\n  execution_target: local\n  working_directory: "+repoRoot+"\n  workspace_kind: git-repo-root\n"+
		"evidence:\n"+
		"  attempts:\n"+
		"    - command: anton task-state pulse\n"+
		"      at: 2026-04-17T00:05:00Z\n"+
		"      outcome: pulse\n"+
		"      validated: false\n"+
		"  validations:\n"+
		"    - command: anton task-state close\n"+
		"      at: 2026-04-17T00:09:00Z\n"+
		"      outcome: review ready\n"+
		"      validated: true\n"+
		"closure:\n  finish_state: review\n  next_step: request reviewer feedback\n  blockers: []\n  expected_deliverables: [\"PR\"]\n",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withHandoffWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"build", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID                 string `json:"task_id"`
			Objective              string `json:"objective"`
			Lifecycle              string `json:"lifecycle"`
			ValidationReceiptCount int    `json:"validation_receipt_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload")
	}
	if payload.Data.TaskID != "demo_task" {
		t.Fatalf("task_id = %q", payload.Data.TaskID)
	}
	if payload.Data.Objective != "Ship stable handoff pack output." {
		t.Fatalf("objective = %q", payload.Data.Objective)
	}
	if payload.Data.Lifecycle != "review" {
		t.Fatalf("lifecycle = %q", payload.Data.Lifecycle)
	}
	if payload.Data.ValidationReceiptCount != 1 {
		t.Fatalf("validation_receipt_count = %d", payload.Data.ValidationReceiptCount)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHandoffBuildFailsWhenStatusMissing(t *testing.T) {
	repoRoot := makeHandoffTempRepoRoot(t)
	writeHandoffFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeHandoffFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	writeHandoffFile(t, filepath.Join(repoRoot, ".anton", "state", "active", "demo_task", "task_plan.md"), "# Task Plan\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withHandoffWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"build", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.OK {
		t.Fatalf("expected failure payload")
	}
	if payload.Error.Code != "handoff-build-failed" {
		t.Fatalf("error code = %q", payload.Error.Code)
	}
}

func withHandoffWorkingDirectory(t *testing.T, path string, fn func() int) int {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("chdir %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	})
	return fn()
}

func makeHandoffTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeHandoffFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeHandoffFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
