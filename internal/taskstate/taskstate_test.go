package taskstate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Andrew0613/Anton/internal/adapter"
)

func TestReadStatusParsesExpectedSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "status.yaml")

	content := "" +
		"version: 1\n" +
		"stable:\n" +
		"  task_id: task-20260416T120000Z\n" +
		"  created_at: 2026-04-16T12:00:00Z\n" +
		"state:\n" +
		"  lifecycle: active\n" +
		"  updated_at: 2026-04-16T12:30:00Z\n" +
		"machine:\n" +
		"  host: devbox\n" +
		"  execution_target: local\n" +
		"  working_directory: /tmp/example\n" +
		"  workspace_kind: plain-directory\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write status.yaml: %v", err)
	}

	snapshot, err := adapter.Default{}.ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}

	if snapshot.TaskID != "task-20260416T120000Z" {
		t.Fatalf("task id = %q", snapshot.TaskID)
	}
}

func TestReadStatusRejectsMissingRequiredFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "status.yaml")

	content := "" +
		"version: 1\n" +
		"stable:\n" +
		"  task_id: \n" +
		"  created_at: 2026-04-16T12:00:00Z\n" +
		"state:\n" +
		"  lifecycle: active\n" +
		"  updated_at: 2026-04-16T12:30:00Z\n" +
		"machine:\n" +
		"  host: devbox\n" +
		"  execution_target: local\n" +
		"  working_directory: /tmp/example\n" +
		"  workspace_kind: plain-directory\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write status.yaml: %v", err)
	}

	if _, err := (adapter.Default{}).ReadStatus(path); err == nil {
		t.Fatalf("ReadStatus should fail for missing task_id")
	}
}

func TestSummarizeBlocksOnMissingOrInvalidFiles(t *testing.T) {
	result := summarize([]fileResult{
		{Status: "existing"},
		{Status: "created"},
		{Status: "missing"},
		{Status: "invalid"},
	})

	if result.Status != statusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, statusBlocked)
	}
	if result.CreatedCount != 1 || result.ExistingCount != 1 || result.MissingCount != 1 || result.InvalidCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", result)
	}
}

func TestValidateBundleCompleteFixture(t *testing.T) {
	bundle := adapter.ResolvedTaskBundle{
		Root: bundleFixturePath("complete"),
		RequiredFiles: []adapter.TaskFile{
			{Name: "task_plan.md"},
			{Name: "findings.md"},
			{Name: "progress.md"},
		},
		StatusFile: "status.yaml",
	}
	results := validateBundle(bundle.Root, bundle)

	for _, result := range results {
		if result.Status != "existing" {
			t.Fatalf("expected existing file result, got %+v", result)
		}
	}
}

func TestValidateBundleIncompleteFixture(t *testing.T) {
	bundle := adapter.ResolvedTaskBundle{
		Root: bundleFixturePath("incomplete"),
		RequiredFiles: []adapter.TaskFile{
			{Name: "task_plan.md"},
			{Name: "findings.md"},
			{Name: "progress.md"},
		},
		StatusFile: "status.yaml",
	}
	results := validateBundle(bundle.Root, bundle)

	missingCount := 0
	for _, result := range results {
		if result.Status == "missing" {
			missingCount++
		}
	}
	if missingCount != 2 {
		t.Fatalf("missing count = %d, want 2; results=%+v", missingCount, results)
	}
}

func TestReadStatusFromFixture(t *testing.T) {
	snapshot, err := adapter.Default{}.ReadStatus(filepath.Join(bundleFixturePath("complete"), "status.yaml"))
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}

	if snapshot.TaskID != "task-20260416T120000Z" {
		t.Fatalf("task id = %q", snapshot.TaskID)
	}
}

func TestTaskStateCheckJSONUsesConfiguredTasksRoot(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/state\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	writeTaskStateFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 because files are intentionally missing", exitCode)
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Config struct {
				Source    string `json:"source"`
				TasksRoot string `json:"tasks_root"`
			} `json:"config"`
			BundleRoot string `json:"bundle_root"`
			StatusPath string `json:"status_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Data.Config.Source != "repo-local anton.yaml" {
		t.Fatalf("config source = %q", payload.Data.Config.Source)
	}
	if payload.Data.Config.TasksRoot != ".anton/state" {
		t.Fatalf("tasks root = %q", payload.Data.Config.TasksRoot)
	}
	wantBundleSuffix := filepath.Join(".anton", "state", "active", "demo_task")
	if !strings.HasSuffix(payload.Data.BundleRoot, wantBundleSuffix) {
		t.Fatalf("bundle root = %q, want suffix %q", payload.Data.BundleRoot, wantBundleSuffix)
	}
	wantStatusSuffix := filepath.Join(".anton", "state", "active", "demo_task", "status.yaml")
	if !strings.HasSuffix(payload.Data.StatusPath, wantStatusSuffix) {
		t.Fatalf("status path = %q, want suffix %q", payload.Data.StatusPath, wantStatusSuffix)
	}
}

func bundleFixturePath(name string) string {
	return filepath.Join("testdata", "bundles", name)
}

func withTaskStateWorkingDirectory(t *testing.T, path string, fn func() int) int {
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

func makeTaskStateTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeTaskStateFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeTaskStateFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
