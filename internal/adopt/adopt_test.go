package adopt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestAnalyzeCleanRepoHasNoGaps(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeAdoptFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	writeAdoptFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md for details.\n")
	writeAdoptFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".anton", "tasks", "active"), 0o755); err != nil {
		t.Fatalf("mkdir active tasks: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	assertAdoptGoldenJSON(t, stdout.Bytes(), "adopt_plan_clean.json", adoptReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanReportsMissingConfigAsAdvisoryGap(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeAdoptFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md for details.\n")
	writeAdoptFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".anton", "tasks", "active"), 0o755); err != nil {
		t.Fatalf("mkdir active tasks: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	assertAdoptGoldenJSON(t, stdout.Bytes(), "adopt_plan_missing_config.json", adoptReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanReportsMissingEntrypointAsAdvisoryGap(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeAdoptFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	writeAdoptFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".anton", "tasks", "active"), 0o755); err != nil {
		t.Fatalf("mkdir active tasks: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	assertAdoptGoldenJSON(t, stdout.Bytes(), "adopt_plan_missing_entrypoint.json", adoptReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanFailsWithInvalidAntonYAML(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeAdoptFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 2\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	assertAdoptGoldenJSON(t, stdout.Bytes(), "adopt_plan_invalid_config.json", adoptReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanUsageErrorJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"plan", "--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertAdoptGoldenJSON(t, stdout.Bytes(), "adopt_plan_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanHumanOutput(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeAdoptFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md for details.\n")
	writeAdoptFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--human"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "Anton adopt plan") || !strings.Contains(stdout.String(), "Gaps") {
		t.Fatalf("unexpected human output:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHarnessInventoryJSONClassifiesHeavyHarness(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeHeavyHarnessFixture(t, repoRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"harness-inventory", "--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr=%s", exitCode, stderr.String())
	}

	assertInventoryGoldenJSON(t, stdout.Bytes(), "adopt_harness_inventory_heavy.json", adoptReplacements(repoRoot))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHarnessInventoryMarkdownOutput(t *testing.T) {
	repoRoot := makeAdoptTempRepoRoot(t)
	writeHeavyHarnessFixture(t, repoRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"harness-inventory", "--format", "markdown"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr=%s", exitCode, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"# Anton Harness Inventory", "## move_to_anton", "## keep_project_local", "## delete_or_deprecate", "## manual_review"} {
		if !strings.Contains(output, want) {
			t.Fatalf("markdown output missing %q:\n%s", want, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func assertAdoptGoldenJSON(t *testing.T, payload []byte, goldenPath string, replacements map[string]string) {
	t.Helper()

	actual := normalizeAdoptJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveAdoptGoldenPath(t, goldenPath))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	expected := normalizeAdoptJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenPath, actual, expected)
	}
}

func normalizeAdoptJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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

	var parsed response
	if err := json.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, normalized)
	}
	canonical, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return fmt.Sprintf("%s\n", canonical)
}

func assertInventoryGoldenJSON(t *testing.T, payload []byte, goldenPath string, replacements map[string]string) {
	t.Helper()

	actual := normalizeInventoryJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveAdoptGoldenPath(t, goldenPath))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	expected := normalizeInventoryJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenPath, actual, expected)
	}
}

func normalizeInventoryJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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

	var parsed inventoryResponse
	if err := json.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, normalized)
	}
	canonical, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return fmt.Sprintf("%s\n", canonical)
}

func resolveAdoptGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}

func adoptReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func withWorkingDirectory(t *testing.T, path string, fn func() int) int {
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

func makeAdoptTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeAdoptFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeHeavyHarnessFixture(t *testing.T, repoRoot string) {
	t.Helper()

	fixtureRoot := resolveAdoptFixturePath(t, "heavy-harness")
	if err := filepath.WalkDir(fixtureRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(fixtureRoot, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeAdoptFile(t, filepath.Join(repoRoot, rel), string(content))
		return nil
	}); err != nil {
		t.Fatalf("copy heavy harness fixture: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repoRoot, ".anton", "tasks", "active"), 0o755); err != nil {
		t.Fatalf("mkdir active tasks: %v", err)
	}
}

func resolveAdoptFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for fixture %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "fixtures", name)
}

func writeAdoptFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
