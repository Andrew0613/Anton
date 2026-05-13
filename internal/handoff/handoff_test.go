package handoff

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestHandoffBuildCodexSourceRedactsSnippet(t *testing.T) {
	repoRoot := makeHandoffTempRepoRoot(t)
	writeCompleteHandoffBundle(t, repoRoot)
	sessionPath := filepath.Join(t.TempDir(), "fake-session.jsonl")
	writeHandoffFile(t, sessionPath, `{"type":"event_msg","payload":{"message":"Decision: continue, api_key=sk-1234567890abcdef should never leak"}}`+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withHandoffWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"build", "--source", "codex", "--session-id", sessionPath, "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Source struct {
				Kind      string `json:"kind"`
				SessionID string `json:"session_id"`
				Path      string `json:"path"`
			} `json:"source"`
			SourceSnippets []struct {
				Text string `json:"text"`
			} `json:"source_snippets"`
			UserDecisions []string `json:"user_decisions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Fatalf("expected success payload")
	}
	if payload.Data.Source.Kind != "codex" || payload.Data.Source.Path == "" {
		t.Fatalf("source = %+v", payload.Data.Source)
	}
	if len(payload.Data.SourceSnippets) == 0 {
		t.Fatalf("expected source snippets")
	}
	text := payload.Data.SourceSnippets[0].Text
	if strings.Contains(text, "sk-1234567890abcdef") {
		t.Fatalf("secret was not redacted: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("redacted marker missing: %s", text)
	}
	if len(payload.Data.UserDecisions) == 0 {
		t.Fatalf("expected decision extraction")
	}
}

func TestHandoffBuildIncludesDirtyGitFiles(t *testing.T) {
	repoRoot := makeRealGitRepoRoot(t)
	writeCompleteHandoffBundle(t, repoRoot)
	writeHandoffFile(t, filepath.Join(repoRoot, "tracked.txt"), "initial\n")
	runGit(t, repoRoot, "add", "tracked.txt")
	writeHandoffFile(t, filepath.Join(repoRoot, "tracked.txt"), "changed\n")
	writeHandoffFile(t, filepath.Join(repoRoot, "untracked.txt"), "new\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withHandoffWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"build", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Data struct {
			Git struct {
				DirtyTracked []string `json:"dirty_tracked"`
				Untracked    []string `json:"untracked"`
			} `json:"git"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !containsString(payload.Data.Git.DirtyTracked, "tracked.txt") {
		t.Fatalf("dirty_tracked = %#v", payload.Data.Git.DirtyTracked)
	}
	if !containsString(payload.Data.Git.Untracked, "untracked.txt") {
		t.Fatalf("untracked = %#v", payload.Data.Git.Untracked)
	}
}

func TestHandoffBuildUsesAdapterSnapshotForNonAntonStatusSummary(t *testing.T) {
	repoRoot := makeHandoffTempRepoRoot(t)
	writeHandoffFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeHandoffFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	bundleRoot := filepath.Join(repoRoot, "project_progress", "PAWBench", "_legacy_picaworld", "tasks", "active", "demo_topic_task")
	writeHandoffFile(t, filepath.Join(bundleRoot, "task_plan.md"), "# Task Plan\n\n## Goal\nResume topic-layer work.\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "findings.md"), "# Findings\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "progress.md"), "# Progress\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "status.yaml"), ""+
		"version: 1\n"+
		"task:\n"+
		"  id: demo_topic_task\n"+
		"  topic: PAWBench/_legacy_picaworld\n"+
		"  title: Topic task\n"+
		"  lifecycle: blocked\n"+
		"  phase: implementation\n"+
		"  owner: agent\n"+
		"  last_updated: 2026-05-13\n"+
		"execution:\n"+
		"  worktree: "+repoRoot+"\n"+
		"  branch: main\n"+
		"  cwd: "+repoRoot+"\n"+
		"  scope_paths: []\n"+
		"  batch_rjob: false\n"+
		"environment:\n"+
		"  machine_type: local\n"+
		"  host: test\n"+
		"  proxy: unknown\n"+
		"  python: python3\n"+
		"services: []\n"+
		"state:\n"+
		"  done: []\n"+
		"  in_progress: []\n"+
		"  blockers: [needs user decision]\n"+
		"  next_actions: [ask user]\n"+
		"  needs_user: []\n",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withHandoffWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"build", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_topic_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Data struct {
			TaskID      string            `json:"task_id"`
			Lifecycle   string            `json:"lifecycle"`
			Blockers    int               `json:"blocker_count"`
			TaskStatus  taskStatusSummary `json:"task_status"`
			BlockerList []string          `json:"blockers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Data.TaskID != "demo_topic_task" || payload.Data.Lifecycle != "blocked" || payload.Data.Blockers != 1 {
		t.Fatalf("unexpected top-level summary: %+v", payload.Data)
	}
	if payload.Data.TaskStatus.TaskID != "demo_topic_task" || payload.Data.TaskStatus.Lifecycle != "blocked" || payload.Data.TaskStatus.NextStep != "ask user" {
		t.Fatalf("task_status = %+v", payload.Data.TaskStatus)
	}
	if len(payload.Data.BlockerList) != 0 {
		t.Fatalf("non-Anton blocker details should not be parsed by handoff: %#v", payload.Data.BlockerList)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPersistResultsDryRunReportsCopyPlanAndWritesNothing(t *testing.T) {
	worktreeRoot := t.TempDir()
	runDir := t.TempDir()
	writeHandoffFile(t, filepath.Join(runDir, "results", "summary.json"), `{"ok":true}`+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"persist-results", "--worktree-root", worktreeRoot, "--run-dir", runDir, "--dry-run", "--json"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun          bool   `json:"dry_run"`
			WouldWrite      bool   `json:"would_write"`
			FileCount       int    `json:"file_count"`
			DestinationRoot string `json:"destination_root"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if !payload.OK || !payload.Data.DryRun || payload.Data.WouldWrite {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.FileCount != 1 {
		t.Fatalf("file_count = %d", payload.Data.FileCount)
	}
	if _, err := os.Stat(payload.Data.DestinationRoot); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote destination root or stat failed: %v", err)
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

func makeRealGitRepoRoot(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	return repoRoot
}

func writeCompleteHandoffBundle(t *testing.T, repoRoot string) {
	t.Helper()

	writeHandoffFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeHandoffFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	bundleRoot := filepath.Join(repoRoot, ".anton", "state", "active", "demo_task")
	writeHandoffFile(t, filepath.Join(bundleRoot, "task_plan.md"), "# Task Plan\n\n## Goal\nShip stable handoff pack output.\n\n- Decision: keep handoff compact.\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "findings.md"), "# Findings\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "progress.md"), "# Progress\n")
	writeHandoffFile(t, filepath.Join(bundleRoot, "status.yaml"), ""+
		"version: 1\n"+
		"stable:\n  task_id: demo_task\n  created_at: 2026-04-17T00:00:00Z\n"+
		"state:\n  lifecycle: review\n  updated_at: 2026-04-17T00:10:00Z\n"+
		"machine:\n  host: test\n  execution_target: local\n  working_directory: "+repoRoot+"\n  workspace_kind: git-repo-root\n"+
		"evidence:\n"+
		"  attempts: []\n"+
		"  validations:\n"+
		"    - command: go test ./internal/handoff\n"+
		"      at: 2026-04-17T00:09:00Z\n"+
		"      outcome: pass\n"+
		"      validated: true\n"+
		"closure:\n  finish_state: review\n  next_step: request reviewer feedback\n  blockers: [\"waiting for user decision on release timing\"]\n  expected_deliverables: [\"PR\"]\n",
	)
}

func runGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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
