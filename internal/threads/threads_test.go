package threads

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

func TestScopeWarningRequiresProject(t *testing.T) {
	if warning := scopeWarning("Anton"); warning != "" {
		t.Fatalf("scopeWarning returned %q for project-scoped request", warning)
	}
	if warning := scopeWarning(""); warning == "" {
		t.Fatalf("scopeWarning should warn when project scope is missing")
	}
}

func TestFindOnPathReturnsExecutableCandidate(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "codex-threads")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	got := findOnPath("codex-threads", root)
	if got != binary {
		t.Fatalf("findOnPath returned %q, want %q", got, binary)
	}
}

func TestParseOptionsRejectsUnexpectedArguments(t *testing.T) {
	if _, err := parseOptions([]string{"--bad-flag"}, true, 20); err == nil {
		t.Fatalf("parseOptions should reject unexpected arguments")
	}
}

func TestThreadsUseAdapterProjectResolution(t *testing.T) {
	context := adapter.Context{
		WorkingDirectory: "/tmp/Anton",
		RepositoryRoot:   "/tmp/Anton",
	}

	project := adapter.Default{}.ResolveThreadsProject(context, nil, "")
	if project.Name != "Anton" || project.Source != "repo-root" {
		t.Fatalf("adapter project = %#v", project)
	}
}

func TestThreadsRecentHonorsConfigProjectStrategy(t *testing.T) {
	cases := []struct {
		name                string
		strategy            string
		wantProject         string
		wantProjectFromRepo bool
		wantProjectOmitted  bool
	}{
		{
			name:                "repo-root-default",
			strategy:            "repo-root",
			wantProjectFromRepo: true,
			wantProjectOmitted:  false,
		},
		{
			name:               "none-strategy",
			strategy:           "none",
			wantProject:        "",
			wantProjectOmitted: true,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			repoRoot := makeThreadsTempRepoRoot(t, testCase.strategy)
			argsFile := filepath.Join(repoRoot, "codex-threads-args.txt")
			binDir := filepath.Join(repoRoot, "bin")
			fakeBinary := filepath.Join(binDir, "codex-threads")
			writeThreadsFile(t, fakeBinary, "#!/bin/sh\n"+
				"echo \"$@\" > \"$FAKE_ARGS_PATH\"\n"+
				"echo '{\"ok\":true}'\n",
			)
			if err := os.Chmod(fakeBinary, 0o755); err != nil {
				t.Fatalf("chmod fake binary: %v", err)
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			env := []string{
				"PATH=" + binDir,
				"HOME=" + repoRoot,
				"FAKE_ARGS_PATH=" + argsFile,
			}
			exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
				return Run([]string{"recent", "--json", "--limit", "5"}, &stdout, &stderr, env)
			})
			if exitCode != 0 {
				t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
			}

			var payload struct {
				OK   bool `json:"ok"`
				Data struct {
					Adapter struct {
						ConfigSource string `json:"config_source"`
						Project      string `json:"project"`
					} `json:"adapter"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("decode payload: %v\n%s", err, stdout.String())
			}
			if !payload.OK {
				t.Fatalf("expected success payload")
			}
			if payload.Data.Adapter.ConfigSource != "repo-local anton.yaml" {
				t.Fatalf("config source = %q", payload.Data.Adapter.ConfigSource)
			}
			expectedProject := testCase.wantProject
			if testCase.wantProjectFromRepo {
				expectedProject = filepath.Base(repoRoot)
			}
			if payload.Data.Adapter.Project != expectedProject {
				t.Fatalf("project = %q, want %q", payload.Data.Adapter.Project, expectedProject)
			}

			argsContent, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("read args file: %v", err)
			}
			hasProjectFlag := strings.Contains(string(argsContent), "--project")
			if testCase.wantProjectOmitted && hasProjectFlag {
				t.Fatalf("args should not include --project: %q", string(argsContent))
			}
			if !testCase.wantProjectOmitted && !hasProjectFlag {
				t.Fatalf("args should include --project: %q", string(argsContent))
			}
		})
	}
}

func TestThreadsDoctorJSONContract(t *testing.T) {
	repoRoot := makeThreadsTempRepoRoot(t, "repo-root")
	binDir := filepath.Join(repoRoot, "bin")
	fakeBinary := filepath.Join(binDir, "codex-threads")
	writeThreadsFile(t, fakeBinary, "#!/bin/sh\n"+
		"echo '{\"ok\":true,\"source\":\"doctor\"}'\n",
	)
	if err := os.Chmod(fakeBinary, 0o755); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"doctor", "--json"}, &stdout, &stderr, []string{
			"PATH=" + binDir,
			"HOME=" + repoRoot,
		})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	replacements := threadsReplacements(repoRoot)
	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_doctor_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestThreadsRecentJSONContract(t *testing.T) {
	repoRoot := makeThreadsTempRepoRoot(t, "repo-root")
	binDir := filepath.Join(repoRoot, "bin")
	fakeBinary := filepath.Join(binDir, "codex-threads")
	writeThreadsFile(t, fakeBinary, "#!/bin/sh\n"+
		"echo '{\"ok\":true,\"items\":[{\"id\":\"thread-1\"}]}'\n",
	)
	if err := os.Chmod(fakeBinary, 0o755); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"recent", "--json", "--limit", "5", "--project", "Anton"}, &stdout, &stderr, []string{
			"PATH=" + binDir,
			"HOME=" + repoRoot,
		})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	replacements := threadsReplacements(repoRoot)
	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_recent_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestThreadsInsightsJSONContract(t *testing.T) {
	repoRoot := makeThreadsTempRepoRoot(t, "repo-root")
	binDir := filepath.Join(repoRoot, "bin")
	fakeBinary := filepath.Join(binDir, "codex-threads")
	writeThreadsFile(t, fakeBinary, "#!/bin/sh\n"+
		"echo '{\"ok\":true,\"insights\":{\"sessions\":3}}'\n",
	)
	if err := os.Chmod(fakeBinary, 0o755); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"insights", "--json", "--limit", "7", "--project", "Anton"}, &stdout, &stderr, []string{
			"PATH=" + binDir,
			"HOME=" + repoRoot,
		})
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}

	replacements := threadsReplacements(repoRoot)
	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_insights_success.json", replacements)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestThreadsRecentUsageErrorExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"recent", "--json", "--bad-flag"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_usage_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestThreadsRecentRuntimeMissingBinaryExitCode(t *testing.T) {
	repoRoot := makeThreadsTempRepoRoot(t, "repo-root")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"recent", "--json"}, &stdout, &stderr, []string{
			"PATH=",
			"HOME=" + repoRoot,
		})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_runtime_missing_binary.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestThreadsDoctorRuntimeDecodeErrorExitCode(t *testing.T) {
	repoRoot := makeThreadsTempRepoRoot(t, "repo-root")
	binDir := filepath.Join(repoRoot, "bin")
	fakeBinary := filepath.Join(binDir, "codex-threads")
	writeThreadsFile(t, fakeBinary, "#!/bin/sh\n"+
		"echo '{'\n",
	)
	if err := os.Chmod(fakeBinary, 0o755); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withThreadsWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"doctor", "--json"}, &stdout, &stderr, []string{
			"PATH=" + binDir,
			"HOME=" + repoRoot,
		})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	assertThreadsGoldenJSON(t, stdout.Bytes(), "threads_runtime_decode_error.json", nil)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func threadsReplacements(repoRoot string) map[string]string {
	return map[string]string{
		filepath.Clean(repoRoot):                            "<REPO_ROOT>",
		filepath.Clean(filepath.Join("/private", repoRoot)): "<REPO_ROOT>",
	}
}

func assertThreadsGoldenJSON(t *testing.T, payload []byte, goldenName string, replacements map[string]string) {
	t.Helper()

	actual := normalizeThreadsJSON(t, payload, replacements)
	expectedBytes, err := os.ReadFile(resolveThreadsGoldenPath(t, goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	expected := normalizeThreadsJSON(t, expectedBytes, nil)
	if actual != expected {
		t.Fatalf("json contract mismatch for %s\n--- actual ---\n%s\n--- expected ---\n%s", goldenName, actual, expected)
	}
}

func normalizeThreadsJSON(t *testing.T, payload []byte, replacements map[string]string) string {
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

func resolveThreadsGoldenPath(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path for golden file %s", name)
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden", name)
}

func withThreadsWorkingDirectory(t *testing.T, path string, fn func() int) int {
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

func makeThreadsTempRepoRoot(t *testing.T, strategy string) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeThreadsFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeThreadsFile(t, filepath.Join(repoRoot, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: "+strategy+"\n",
	)
	writeThreadsFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	return repoRoot
}

func writeThreadsFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
