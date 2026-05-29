package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Andrew0613/Anton/internal/adapter"
)

func TestLoadInventoryAndResolveTask(t *testing.T) {
	repo := makeStateRepo(t)
	writeStateFile(t, filepath.Join(repo, "docs", "state", "tasks", "task-a.yaml"), ""+
		"task_id: task-a\n"+
		"topic: Tooling\n"+
		"lane: implementation\n"+
		"lifecycle: active\n"+
		"workspace:\n  path: .\n")
	writeStateFile(t, filepath.Join(repo, "docs", "state", "tasks", "task-b.yaml"), ""+
		"task_id: task-b\n"+
		"lifecycle: done\n")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, false)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if len(inventory.Tasks) != 2 || len(inventory.Active) != 1 {
		t.Fatalf("inventory = %+v", inventory)
	}
	record, issues, err := ResolveTask(resolved, "", false)
	if err != nil {
		t.Fatalf("ResolveTask error: %v", err)
	}
	if record.TaskID != "task-a" {
		t.Fatalf("resolved task = %+v", record)
	}
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func TestLoadInventoryDualReadWarningWhenLegacyMissing(t *testing.T) {
	repo := makeStateRepo(t)
	writeStateFile(t, filepath.Join(repo, "docs", "state", "tasks", "task-a.yaml"), "task_id: task-a\nlifecycle: active\n")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	found := false
	for _, issue := range inventory.Issues {
		if issue.Code == "state-dual-read-missing-legacy" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dual-read warning, got %+v", inventory.Issues)
	}
}

func makeStateRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "anton.yaml"), []byte("version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n"), 0o644); err != nil {
		t.Fatalf("write anton.yaml: %v", err)
	}
	return root
}

func writeStateFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func resolveRepo(t *testing.T, root string) adapter.Resolved {
	t.Helper()
	resolved, err := adapter.Resolve(root, nil)
	if err != nil {
		t.Fatalf("adapter.Resolve: %v", err)
	}
	return resolved
}
