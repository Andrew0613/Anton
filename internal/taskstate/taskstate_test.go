package taskstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_check_blocked_missing.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateInitJSONContract(t *testing.T) {
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
		return Run([]string{"init", "--json"}, &stdout, &stderr, []string{"ANTON_TASK_ID=demo_task"})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_init_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckJSONContractAfterInit(t *testing.T) {
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
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"check", "--json"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_check_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStatePulseJSONContract(t *testing.T) {
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
		env := []string{"ANTON_TASK_ID=demo_task"}
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		return Run([]string{"pulse", "--json"}, &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_pulse_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckUsageErrorExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"check", "--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskStateCheckRuntimeFailureExitCode(t *testing.T) {
	repoRoot := makeTaskStateTempRepoRoot(t)
	writeTaskStateFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 2\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withTaskStateWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	replacements := taskStateReplacements(repoRoot)
	assertTaskStateGoldenJSON(t, stdout.Bytes(), "task_state_runtime_error.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func taskStateReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func assertTaskStateGoldenJSON(t *testing.T, payload []byte, goldenName string, replacements map[string]string) {
	t.Helper()

	actual := normalizeTaskStateJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveTaskStateGoldenPath(t, goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	expected := normalizeTaskStateJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenName, actual, expected)
	}
}

func normalizeTaskStateJSON(t *testing.T, payload []byte, replacements map[string]string) string {
	t.Helper()

	normalized := string(payload)
	keys := make([]string, 0, len(replacements))
	for old := range replacements {
		keys = append(keys, old)
	}
	sort.Slice(keys, func(i int, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	for _, old := range keys {
		normalized = strings.ReplaceAll(normalized, old, replacements[old])
	}

	var value any
	if err := json.Unmarshal([]byte(normalized), &value); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, normalized)
	}

	canonical, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return fmt.Sprintf("%s\n", canonical)
}

func resolveTaskStateGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
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
