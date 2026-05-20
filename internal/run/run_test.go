package run

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
	"time"
)

const fixedNow = "2026-05-20T00:00:00Z"

func TestNewManifestValidatesChecklistStatus(t *testing.T) {
	manifest, err := NewManifest("demo_task", mustParseTime(t, fixedNow))
	if err != nil {
		t.Fatalf("NewManifest returned error: %v", err)
	}
	manifest.Checklist = append(manifest.Checklist, ChecklistItem{
		ID:     "u1",
		Title:  "Do the thing",
		Status: "maybe",
		Notes:  []string{},
	})
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("Validate error = %v, want checklist status values", err)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	bundleRoot := filepath.Join(root, ".anton", "tasks", "active", "demo_task")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}

	store := NewStore(bundleRoot)
	manifest, err := NewManifest("demo_task", mustParseTime(t, fixedNow))
	if err != nil {
		t.Fatalf("NewManifest returned error: %v", err)
	}
	if err := manifest.AddChecklistItem("u1", "Wire store", mustParseTime(t, fixedNow)); err != nil {
		t.Fatalf("AddChecklistItem returned error: %v", err)
	}
	if err := store.Save(manifest); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.TaskID != "demo_task" || len(loaded.Checklist) != 1 {
		t.Fatalf("loaded manifest = %+v", loaded)
	}
}

func TestStoreLoadForTaskRejectsMismatchedTaskID(t *testing.T) {
	root := t.TempDir()
	bundleRoot := filepath.Join(root, ".anton", "tasks", "active", "demo_task")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}

	store := NewStore(bundleRoot)
	manifest, err := NewManifest("other_task", mustParseTime(t, fixedNow))
	if err != nil {
		t.Fatalf("NewManifest returned error: %v", err)
	}
	if err := store.Save(manifest); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if _, err := store.LoadForTask("demo_task"); err == nil || !strings.Contains(err.Error(), "does not match active task") {
		t.Fatalf("LoadForTask error = %v, want task mismatch", err)
	}
}

func TestStoreRequiresExistingTaskBundle(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), ".anton", "tasks", "active", "missing"))
	manifest, err := NewManifest("missing", mustParseTime(t, fixedNow))
	if err != nil {
		t.Fatalf("NewManifest returned error: %v", err)
	}
	if err := store.Save(manifest); err == nil || !strings.Contains(err.Error(), "task bundle root is required") {
		t.Fatalf("Save error = %v, want missing bundle error", err)
	}
}

func TestStoreRefusesSymlinkManifest(t *testing.T) {
	root := t.TempDir()
	bundleRoot := filepath.Join(root, ".anton", "tasks", "active", "demo_task")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	target := filepath.Join(root, "outside.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(bundleRoot, ManifestFilename)); err != nil {
		t.Fatalf("symlink manifest: %v", err)
	}

	manifest, err := NewManifest("demo_task", mustParseTime(t, fixedNow))
	if err != nil {
		t.Fatalf("NewManifest returned error: %v", err)
	}
	if err := NewStore(bundleRoot).Save(manifest); err == nil || !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("Save error = %v, want symlink refusal", err)
	}
}

func TestRunInitJSONContract(t *testing.T) {
	repoRoot := makeRunTempRepoRoot(t)
	makeRunBundle(t, repoRoot, "demo_task")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withRunWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, runEnv("demo_task"))
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertRunGoldenJSON(t, stdout.Bytes(), "run_init_success.json", runReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunTaskAuditCloseJSONContract(t *testing.T) {
	repoRoot := makeRunTempRepoRoot(t)
	makeRunBundle(t, repoRoot, "demo_task")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withRunWorkingDirectory(t, repoRoot, func() int {
		env := runEnv("demo_task")
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		commands := [][]string{
			{"task", "add", "--json", "--id", "u1", "--title", "Add run manifest package"},
			{"task", "set", "--json", "--id", "u1", "--status", "done", "--note", "implemented"},
			{"audit", "add", "--json", "--kind", "test", "--name", "unit", "--status", "passed", "--summary", "go test ./internal/run", "--receipt-path", ".anton/tasks/active/demo_task/receipts/unit.json"},
			{"close", "--json", "--status", "review", "--summary", "ready for integration"},
		}
		for _, command := range commands[:len(commands)-1] {
			if code := Run(command, io.Discard, io.Discard, env); code != 0 {
				t.Fatalf("%v exit code = %d, want 0", command, code)
			}
		}
		return Run(commands[len(commands)-1], &stdout, &stderr, env)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	assertRunGoldenJSON(t, stdout.Bytes(), "run_close_success.json", runReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunInitRequiresExplicitTaskIdentity(t *testing.T) {
	repoRoot := makeRunTempRepoRoot(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withRunWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"init", "--json"}, &stdout, &stderr, []string{"ANTON_RUN_NOW=" + fixedNow})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Error == nil || payload.Error.Code != "task-identity-required" {
		t.Fatalf("payload = %+v", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunTaskSetRejectsInvalidChecklistStatus(t *testing.T) {
	repoRoot := makeRunTempRepoRoot(t)
	makeRunBundle(t, repoRoot, "demo_task")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withRunWorkingDirectory(t, repoRoot, func() int {
		env := runEnv("demo_task")
		if code := Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("init exit code = %d, want 0", code)
		}
		if code := Run([]string{"task", "add", "--json", "--id", "u1", "--title", "Do work"}, io.Discard, io.Discard, env); code != 0 {
			t.Fatalf("task add exit code = %d, want 0", code)
		}
		return Run([]string{"task", "set", "--json", "--id", "u1", "--status", "maybe"}, &stdout, &stderr, env)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	assertRunGoldenJSON(t, stdout.Bytes(), "run_task_set_invalid_status.json", runReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func makeRunTempRepoRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeRunFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeRunFile(t, filepath.Join(root, "AGENTS.md"), "# Entry\n")
	writeRunFile(t, filepath.Join(root, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	return root
}

func makeRunBundle(t *testing.T, repoRoot string, taskID string) string {
	t.Helper()

	bundleRoot := filepath.Join(repoRoot, ".anton", "tasks", "active", taskID)
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	return bundleRoot
}

func writeRunFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runEnv(taskID string) []string {
	return []string{"ANTON_TASK_ID=" + taskID, "ANTON_RUN_NOW=" + fixedNow}
}

func withRunWorkingDirectory(t *testing.T, dir string, fn func() int) int {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	return fn()
}

func runReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func assertRunGoldenJSON(t *testing.T, payload []byte, goldenName string, replacements map[string]string) {
	t.Helper()

	actual := normalizeRunJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveRunGoldenPath(t, goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	expected := normalizeRunJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenName, actual, expected)
	}
}

func normalizeRunJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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

func resolveRunGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}
