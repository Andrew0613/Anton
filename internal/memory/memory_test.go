package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestMemoryStatusMissingJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"status", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_status_missing.json", memoryReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryStatusFreshJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)
	writeMemoryEvent(t, repoRoot, Event{
		SchemaVersion: eventSchemaVersion,
		Key:           "handoff.next_step",
		Value:         "finish memory command package",
		Source:        "docs/plans/2026-05-08-006-feat-anton-memory-surface-plan.md",
		Freshness:     "2099-01-01T00:00:00Z",
		Confidence:    "high",
		Author:        "agent",
		RecordedAt:    "2099-01-01T00:00:00Z",
		Advisory:      true,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"status", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_status_fresh.json", memoryReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryStatusStaleJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)
	writeMemoryEvent(t, repoRoot, Event{
		SchemaVersion: eventSchemaVersion,
		Key:           "handoff.next_step",
		Value:         "old handoff note",
		Source:        "chat",
		Freshness:     "2000-01-01T00:00:00Z",
		Confidence:    "medium",
		Author:        "agent",
		RecordedAt:    "2000-01-01T00:00:00Z",
		Advisory:      true,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"status", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_status_stale.json", memoryReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryConflictEntrypointJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)
	writeMemoryEvent(t, repoRoot, Event{
		SchemaVersion: eventSchemaVersion,
		Key:           "entrypoint.path",
		Value:         "README.md",
		Source:        "chat",
		Freshness:     "2099-01-01T00:00:00Z",
		Confidence:    "low",
		Author:        "agent",
		RecordedAt:    "2099-01-01T00:00:00Z",
		Advisory:      true,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"status", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_conflict_entrypoint.json", memoryReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryCorruptJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)
	writeMemoryRaw(t, repoRoot, "{bad json\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"status", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_corrupt.json", memoryReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, ".anton", "memory", "events.jsonl"))
	if err != nil {
		t.Fatalf("read corrupt memory log: %v", err)
	}
	if string(content) != "{bad json\n" {
		t.Fatalf("corrupt memory log was modified: %q", string(content))
	}
}

func TestMemoryUpdateSuccessJSON(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{
			"update",
			"--json",
			"--key", "handoff.next_step",
			"--value", "wire memory command in app layer",
			"--source", "docs/plans/2026-05-08-006-feat-anton-memory-surface-plan.md",
			"--freshness", "2099-01-01T00:00:00Z",
			"--confidence", "high",
			"--author", "agent",
		}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_update_success.json", memoryReplacements(repoRoot))
	content, err := os.ReadFile(filepath.Join(repoRoot, ".anton", "memory", "events.jsonl"))
	if err != nil {
		t.Fatalf("read memory log: %v", err)
	}
	if lines := strings.Count(string(content), "\n"); lines != 1 {
		t.Fatalf("memory update should append one JSONL record, got %d lines: %s", lines, string(content))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryUpdateAppendsDuplicates(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)

	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		args := []string{
			"update",
			"--key", "handoff.next_step",
			"--value", "continue",
			"--source", "chat",
			"--freshness", "2099-01-01T00:00:00Z",
		}
		if code := Run(args, &bytes.Buffer{}, &bytes.Buffer{}, []string{"USER=tester"}); code != 0 {
			t.Fatalf("first update exit code = %d", code)
		}
		return Run(args, &bytes.Buffer{}, &bytes.Buffer{}, []string{"USER=tester"})
	})
	if exitCode != 0 {
		t.Fatalf("second update exit code = %d", exitCode)
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, ".anton", "memory", "events.jsonl"))
	if err != nil {
		t.Fatalf("read memory log: %v", err)
	}
	if lines := strings.Count(string(content), "\n"); lines != 2 {
		t.Fatalf("duplicate update should append two records, got %d lines: %s", lines, string(content))
	}
}

func TestMemoryStatusUsageErrorJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"status", "--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertMemoryGoldenJSON(t, stdout.Bytes(), "memory_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMemoryUpdateRejectsSymlinkedLog(t *testing.T) {
	repoRoot := makeMemoryTempRepoRoot(t)
	target := filepath.Join(repoRoot, "outside.jsonl")
	if err := os.WriteFile(target, []byte(""), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	memoryDir := filepath.Join(repoRoot, ".anton", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(memoryDir, "events.jsonl")); err != nil {
		t.Fatalf("symlink memory log: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withMemoryWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"update", "--json", "--key", "k", "--value", "v", "--source", "test"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "symlinked memory log") {
		t.Fatalf("stdout missing symlink error: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func writeMemoryEvent(t *testing.T, repoRoot string, event Event) {
	t.Helper()

	content, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal memory event: %v", err)
	}
	writeMemoryRaw(t, repoRoot, string(content)+"\n")
}

func writeMemoryRaw(t *testing.T, repoRoot string, content string) {
	t.Helper()

	path := filepath.Join(repoRoot, ".anton", "memory", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir memory dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write memory log: %v", err)
	}
}

func makeMemoryTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeMemoryFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeMemoryFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	writeMemoryFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	return repoRoot
}

func writeMemoryFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withMemoryWorkingDirectory(t *testing.T, path string, fn func() int) int {
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

func memoryReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func assertMemoryGoldenJSON(t *testing.T, payload []byte, goldenName string, replacements map[string]string) {
	t.Helper()

	actual := normalizeMemoryJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveMemoryGoldenPath(t, goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	expected := normalizeMemoryJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenName, actual, expected)
	}
}

func normalizeMemoryJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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
	normalized = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`).ReplaceAllString(normalized, "<TIMESTAMP>")

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

func resolveMemoryGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}
