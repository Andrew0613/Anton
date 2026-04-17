package doctor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	writeDoctorFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"--json"}, &stdout, &stderr, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")})
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (degraded checks expected)", exitCode)
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Config struct {
				Source string `json:"source"`
			} `json:"config"`
			Checks []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.Data.Config.Source != "built-in defaults" {
		t.Fatalf("config source = %q", payload.Data.Config.Source)
	}
	found := false
	for _, item := range payload.Data.Checks {
		if item.Name == "anton-config" {
			found = true
			if item.Status != statusDegraded {
				t.Fatalf("anton-config status = %q, want %q", item.Status, statusDegraded)
			}
		}
	}
	if !found {
		t.Fatalf("expected anton-config check")
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

	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, stdout.String())
	}
	if payload.OK {
		t.Fatalf("expected failure payload")
	}
	if payload.Error.Code != "doctor-failed" {
		t.Fatalf("error code = %q", payload.Error.Code)
	}
	if !strings.Contains(payload.Error.Message, "invalid anton config at") {
		t.Fatalf("error message = %q", payload.Error.Message)
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
