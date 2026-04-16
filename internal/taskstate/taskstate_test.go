package taskstate

import (
	"os"
	"path/filepath"
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

func bundleFixturePath(name string) string {
	return filepath.Join("testdata", "bundles", name)
}
