package task

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskResolveAndList(t *testing.T) {
	repo := makeTaskRepo(t)
	writeTaskFile(t, filepath.Join(repo, "docs", "state", "tasks", "task-a.yaml"), "task_id: task-a\nlifecycle: active\n")
	writeTaskFile(t, filepath.Join(repo, "docs", "state", "tasks", "task-b.yaml"), "task_id: task-b\nlifecycle: done\n")

	var resolveStdout bytes.Buffer
	var resolveStderr bytes.Buffer
	resolveExit := withTaskWD(t, repo, func() int {
		return Run([]string{"resolve", "--json"}, &resolveStdout, &resolveStderr, nil)
	})
	if resolveExit != 0 {
		t.Fatalf("resolve exit = %d stdout=%s stderr=%s", resolveExit, resolveStdout.String(), resolveStderr.String())
	}
	var resolvePayload struct {
		Data struct {
			Task struct {
				TaskID string `json:"task_id"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolveStdout.Bytes(), &resolvePayload); err != nil {
		t.Fatalf("decode resolve: %v", err)
	}
	if resolvePayload.Data.Task.TaskID != "task-a" {
		t.Fatalf("resolve payload = %s", resolveStdout.String())
	}

	var listStdout bytes.Buffer
	var listStderr bytes.Buffer
	listExit := withTaskWD(t, repo, func() int {
		return Run([]string{"list", "--state", "active", "--json"}, &listStdout, &listStderr, nil)
	})
	if listExit != 0 {
		t.Fatalf("list exit = %d stdout=%s stderr=%s", listExit, listStdout.String(), listStderr.String())
	}
	var listPayload struct {
		Data struct {
			Inventory struct {
				Total int `json:"total"`
			} `json:"inventory"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listStdout.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listPayload.Data.Inventory.Total != 1 {
		t.Fatalf("list payload = %s", listStdout.String())
	}
}

func TestTaskListDualReadExitsNonzeroOnParityErrors(t *testing.T) {
	repo := makeTaskRepo(t)
	configureTopicLayerTaskRepo(t, repo)
	writeTaskFile(t, filepath.Join(repo, "docs", "state", "tasks", "0062_hard_cut.yaml"), ""+
		"task_id: 0062_hard_cut\n"+
		"topic: Tooling\n"+
		"lifecycle: active\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := withTaskWD(t, repo, func() int {
		return Run([]string{"list", "--state", "active", "--dual-read", "--json"}, &stdout, &stderr, nil)
	})
	if exit != 1 {
		t.Fatalf("list exit = %d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var payload struct {
		Data struct {
			Issues []struct {
				Code  string `json:"code"`
				Level string `json:"level"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if !hasTaskIssue(payload.Data.Issues, "state-dual-read-missing-current-legacy", "error") {
		t.Fatalf("expected actionable parity issue code: %s", stdout.String())
	}
}

func makeTaskRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTaskFile(t, filepath.Join(root, "AGENTS.md"), "# Agents\n")
	writeTaskFile(t, filepath.Join(root, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	return root
}

func configureTopicLayerTaskRepo(t *testing.T, repo string) {
	t.Helper()
	writeTaskFile(t, filepath.Join(repo, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n"+
		"threads:\n  default_project_strategy: repo-root\n")
}

func writeTaskFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withTaskWD(t *testing.T, dir string, fn func() int) int {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
	return fn()
}

func hasTaskIssue(issues []struct {
	Code  string `json:"code"`
	Level string `json:"level"`
}, code string, level string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Level == level {
			return true
		}
	}
	return false
}
