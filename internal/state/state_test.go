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
	if !hasStateIssue(inventory.Issues, "state-dual-read-missing-current-legacy", "error") {
		t.Fatalf("expected dual-read error, got %+v", inventory.Issues)
	}
}

func TestLoadInventoryDualReadTopicLayerBidirectionalParityPasses(t *testing.T) {
	repo := makeStateRepo(t)
	configureTopicLayerStateRepo(t, repo)
	writeStateFile(t, filepath.Join(repo, "docs", "state", "tasks", "0062_hard_cut.yaml"), ""+
		"task_id: 0062_hard_cut\n"+
		"topic: Tooling\n"+
		"lifecycle: active\n")
	writeLegacyPhysEditStatus(t, repo, "Tooling", "0062_hard_cut", "active", "completed")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if len(errorStateIssues(inventory.Issues)) != 0 {
		t.Fatalf("unexpected parity errors: %+v", inventory.Issues)
	}
	if len(inventory.LegacyActive) != 1 || inventory.LegacyActive[0].Classification != "current_active" {
		t.Fatalf("legacy active inventory = %+v", inventory.LegacyActive)
	}
}

func TestLoadInventoryDualReadTopicLayerAcceptsLegacyProjectProgressTopicPrefix(t *testing.T) {
	repo := makeStateRepo(t)
	configureTopicLayerStateRepo(t, repo)
	writeStateFile(t, filepath.Join(repo, "docs", "state", "tasks", "pawbench.yaml"), ""+
		"task_id: pawbench\n"+
		"topic: PAWBench\n"+
		"lifecycle: active\n")
	writeLegacyPhysEditStatusAt(t, repo, filepath.Join("project_progress", "PAWBench", "tasks", "active", "pawbench", "status.yaml"), "pawbench", "project_progress/PAWBench", "active", "planning")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if len(errorStateIssues(inventory.Issues)) != 0 {
		t.Fatalf("prefixed legacy topic should not break parity: %+v", inventory.Issues)
	}
	if len(inventory.LegacyActive) != 1 || inventory.LegacyActive[0].Topic != "PAWBench" {
		t.Fatalf("legacy active inventory = %+v", inventory.LegacyActive)
	}
}

func TestLoadInventoryDualReadTopicLayerMissingStateProjectionErrors(t *testing.T) {
	repo := makeStateRepo(t)
	configureTopicLayerStateRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "state", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir state tasks: %v", err)
	}
	writeLegacyPhysEditStatus(t, repo, "Tooling", "0062_hard_cut", "active", "planning")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if !hasStateIssue(inventory.Issues, "state-dual-read-missing-state-projection", "error") {
		t.Fatalf("expected missing state projection error, got %+v", inventory.Issues)
	}
}

func TestLoadInventoryDualReadTopicLayerCompletedActiveDirStatusIsNonblocking(t *testing.T) {
	repo := makeStateRepo(t)
	configureTopicLayerStateRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "state", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir state tasks: %v", err)
	}
	writeLegacyPhysEditStatus(t, repo, "Tooling", "0062_hard_cut", "done", "planning")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if len(errorStateIssues(inventory.Issues)) != 0 {
		t.Fatalf("completed legacy active-dir entry should not block parity: %+v", inventory.Issues)
	}
	if len(inventory.LegacyActive) != 0 {
		t.Fatalf("completed legacy status should not be current active: %+v", inventory.LegacyActive)
	}
	if len(inventory.LegacyTasks) != 1 || inventory.LegacyTasks[0].Classification != "legacy_active_dir_inactive" {
		t.Fatalf("expected inactive legacy classification: %+v", inventory.LegacyTasks)
	}
}

func TestLoadInventoryDualReadTopicLayerLegacyPathsAreNonblocking(t *testing.T) {
	repo := makeStateRepo(t)
	configureTopicLayerStateRepo(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "state", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir state tasks: %v", err)
	}
	writeLegacyPhysEditStatusAt(t, repo, filepath.Join("project_progress", "_legacy_Tooling", "tasks", "active", "old_task", "status.yaml"), "old_task", "_legacy_Tooling", "active", "planning")
	writeLegacyPhysEditStatusAt(t, repo, filepath.Join("project_progress", "_archived_worktree_rescue_20260529", "tasks", "active", "rescued_task", "status.yaml"), "rescued_task", "_archived_worktree_rescue_20260529", "active", "planning")
	resolved := resolveRepo(t, repo)

	inventory, err := LoadInventory(resolved, true)
	if err != nil {
		t.Fatalf("LoadInventory error: %v", err)
	}
	if len(errorStateIssues(inventory.Issues)) != 0 {
		t.Fatalf("archive-like legacy paths should not block parity: %+v", inventory.Issues)
	}
	if len(inventory.LegacyActive) != 0 {
		t.Fatalf("archive-like legacy paths should not be current active: %+v", inventory.LegacyActive)
	}
	if len(inventory.LegacyTasks) != 2 {
		t.Fatalf("expected archive/history metadata for both paths: %+v", inventory.LegacyTasks)
	}
	for _, item := range inventory.LegacyTasks {
		if item.Classification != "archive_or_history_only" {
			t.Fatalf("expected archive/history classification: %+v", inventory.LegacyTasks)
		}
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

func configureTopicLayerStateRepo(t *testing.T, repo string) {
	t.Helper()
	writeStateFile(t, filepath.Join(repo, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n"+
		"threads:\n  default_project_strategy: repo-root\n")
}

func writeLegacyPhysEditStatus(t *testing.T, repo string, topic string, taskID string, lifecycle string, phase string) {
	t.Helper()
	writeLegacyPhysEditStatusAt(t, repo, filepath.Join("project_progress", topic, "tasks", "active", taskID, "status.yaml"), taskID, topic, lifecycle, phase)
}

func writeLegacyPhysEditStatusAt(t *testing.T, repo string, relativePath string, taskID string, topic string, lifecycle string, phase string) {
	t.Helper()
	writeStateFile(t, filepath.Join(repo, relativePath), ""+
		"version: 1\n"+
		"task:\n"+
		"  id: "+taskID+"\n"+
		"  topic: "+topic+"\n"+
		"  lifecycle: "+lifecycle+"\n"+
		"  phase: "+phase+"\n")
}

func hasStateIssue(issues []Issue, code string, level string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Level == level {
			return true
		}
	}
	return false
}

func errorStateIssues(issues []Issue) []Issue {
	result := []Issue{}
	for _, issue := range issues {
		if issue.Level == "error" {
			result = append(result, issue)
		}
	}
	return result
}
