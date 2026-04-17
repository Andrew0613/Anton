package doctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Andrew0613/Anton/internal/adapter"
)

func TestSummarizeChecks(t *testing.T) {
	result := summarizeChecks([]check{
		{Name: "a", Status: statusOK},
		{Name: "b", Status: statusDegraded},
		{Name: "c", Status: statusBlocked},
	})

	if result.Status != statusBlocked {
		t.Fatalf("summary status = %q, want blocked", result.Status)
	}
	if result.OKCount != 1 || result.DegradedCount != 1 || result.BlockedCount != 1 {
		t.Fatalf("unexpected counts: %+v", result)
	}
}

func TestCheckAntonConfigReportsLoadedFile(t *testing.T) {
	result := checkAntonConfig(adapter.Config{
		Path:   "/tmp/repo/anton.yaml",
		Loaded: true,
	})

	if result.Status != statusOK {
		t.Fatalf("status = %q, want %q", result.Status, statusOK)
	}
}

func TestCheckAntonConfigReportsMissingFile(t *testing.T) {
	result := checkAntonConfig(adapter.Config{
		Path: "/tmp/repo/anton.yaml",
	})

	if result.Status != statusDegraded {
		t.Fatalf("status = %q, want %q", result.Status, statusDegraded)
	}
	if result.Hint == "" {
		t.Fatalf("expected hint for missing anton.yaml")
	}
}

func TestDoctorJSONReportsBuiltInDefaultsWhenAntonYAMLMissing(t *testing.T) {
	repoRoot := makeDoctorTempRepoRoot(t)
	writeDoctorFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md for details.\n")
	writeDoctorFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")
	binDir := filepath.Join(repoRoot, "bin")
	codexThreads := filepath.Join(binDir, "codex-threads")
	writeDoctorFile(t, codexThreads, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(codexThreads, 0o755); err != nil {
		t.Fatalf("chmod codex-threads: %v", err)
	}
	pathValue := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", pathValue)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"--json"}, &stdout, &stderr, []string{"PATH=" + pathValue, "HOME=" + repoRoot})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (degraded checks expected)", exitCode)
	}

	replacements := doctorReplacements(repoRoot)
	assertDoctorGoldenJSON(t, stdout.Bytes(), "doctor_degraded.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDoctorExplainIncludesRemediation(t *testing.T) {
	repoRoot := makeDoctorTempRepoRoot(t)
	writeDoctorFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md for details.\n")
	writeDoctorFile(t, filepath.Join(repoRoot, "README.md"), "Anton contract uses AGENTS.md and anton.yaml.\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"--explain"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repoRoot})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "Remediation") {
		t.Fatalf("stdout missing Remediation section:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDoctorJSONContextReceiptAcrossWorkspaceKinds(t *testing.T) {
	cases := []struct {
		name              string
		path              string
		wantWorkspaceKind string
	}{
		{
			name:              "repo-root",
			path:              doctorFixturePath("repo-root"),
			wantWorkspaceKind: "git-repo-root",
		},
		{
			name:              "git-subdir",
			path:              filepath.Join(doctorFixturePath("repo-root"), "subdir"),
			wantWorkspaceKind: "git-subdir",
		},
		{
			name:              "git-worktree",
			path:              doctorFixturePath("worktree"),
			wantWorkspaceKind: "git-worktree",
		},
		{
			name:              "plain-directory",
			path:              "",
			wantWorkspaceKind: "plain-directory",
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.path == "" {
				testCase.path = t.TempDir()
				writeDoctorFile(t, filepath.Join(testCase.path, "notes.txt"), "plain workspace")
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := withWorkingDirectory(t, testCase.path, func() int {
				return Run([]string{"--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")})
			})
			if exitCode != 1 && exitCode != 0 {
				t.Fatalf("unexpected exit code %d", exitCode)
			}

			var payload struct {
				OK   bool `json:"ok"`
				Data struct {
					Context struct {
						WorkspaceKind string `json:"workspace_kind"`
					} `json:"context"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("decode payload: %v\n%s", err, stdout.String())
			}
			if payload.Data.Context.WorkspaceKind != testCase.wantWorkspaceKind {
				t.Fatalf("workspace_kind = %q, want %q", payload.Data.Context.WorkspaceKind, testCase.wantWorkspaceKind)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestDoctorJSONFailsWithInvalidAntonYAML(t *testing.T) {
	repoRoot := makeDoctorTempRepoRoot(t)
	writeDoctorFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 2\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d", exitCode)
	}

	replacements := doctorReplacements(repoRoot)
	assertDoctorGoldenJSON(t, stdout.Bytes(), "doctor_invalid_config.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDoctorJSONUsageErrorExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertDoctorGoldenJSON(t, stdout.Bytes(), "doctor_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func doctorReplacements(repoRoot string) map[string]string {
	replacements := map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		replacements[host] = "<HOST>"
	}
	if filesystemType := strings.TrimSpace(detectFilesystemType(repoRoot)); filesystemType != "" && !strings.ContainsRune(filesystemType, os.PathSeparator) {
		replacements[filesystemType] = "<FILESYSTEM_TYPE>"
	}
	return replacements
}

func assertDoctorGoldenJSON(t *testing.T, payload []byte, goldenPath string, replacements map[string]string) {
	t.Helper()

	actual := normalizeDoctorJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveDoctorGoldenPath(t, goldenPath))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	expected := normalizeDoctorJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenPath, actual, expected)
	}
}

func normalizeDoctorJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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

	var parsed report
	if err := json.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, normalized)
	}
	if parsed.Data != nil {
		if strings.TrimSpace(parsed.Data.Environment.Host) != "" {
			parsed.Data.Environment.Host = "<HOST>"
		}
		parsed.Data.Environment.OperatingSystem = "<OPERATING_SYSTEM>"
		parsed.Data.Environment.Architecture = "<ARCHITECTURE>"
		parsed.Data.Environment.FilesystemType = "<FILESYSTEM_TYPE>"
		parsed.Data.Summary.OKCount = -1
		parsed.Data.Summary.DegradedCount = -1
		for index := range parsed.Data.Checks {
			switch parsed.Data.Checks[index].Name {
			case "filesystem-type":
				parsed.Data.Checks[index].Status = "<FILESYSTEM_STATUS>"
				parsed.Data.Checks[index].Detail = "<FILESYSTEM_DETAIL>"
				if strings.TrimSpace(parsed.Data.Checks[index].Hint) != "" {
					parsed.Data.Checks[index].Hint = "<FILESYSTEM_HINT>"
				}
			case "go-toolchain":
				parsed.Data.Checks[index].Status = "<GO_TOOLCHAIN_STATUS>"
				parsed.Data.Checks[index].Detail = "<GO_TOOLCHAIN_DETAIL>"
				if strings.TrimSpace(parsed.Data.Checks[index].Hint) != "" {
					parsed.Data.Checks[index].Hint = "<GO_TOOLCHAIN_HINT>"
				}
			}
		}
		for index := range parsed.Data.Remediation {
			if parsed.Data.Remediation[index].Check == "go-toolchain" {
				parsed.Data.Remediation[index].Severity = "<GO_TOOLCHAIN_STATUS>"
				parsed.Data.Remediation[index].Actions = []string{"<GO_TOOLCHAIN_ACTION>"}
			}
		}
	}

	canonical, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return fmt.Sprintf("%s\n", canonical)
}

func resolveDoctorGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}

func doctorFixturePath(name string) string {
	return filepath.Join("..", "adapter", "testdata", "contexts", name)
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

func makeDoctorTempRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeDoctorFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeDoctorFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
