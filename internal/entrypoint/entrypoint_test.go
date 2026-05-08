package entrypoint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEntrypointCheckSuccess(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n\nRead [README.md](README.md).\n")
	writeFile(t, filepath.Join(repoRoot, "README.md"), "# Repo\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}

	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK {
		t.Fatalf("ok = false, want true: %+v", payload)
	}
	if payload.Data == nil || payload.Data.Summary.Status != statusOK {
		t.Fatalf("summary = %+v", payload.Data)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestEntrypointCheckMissingPrimary(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "README.md"), "# Repo\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	payload := decodeResponse(t, stdout.Bytes())
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.Data == nil || payload.Data.Summary.BlockedCount != 1 {
		t.Fatalf("summary = %+v", payload.Data)
	}
	if payload.Data.Findings[0].Code != "primary-entrypoint-missing" {
		t.Fatalf("finding = %+v", payload.Data.Findings[0])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestEntrypointCheckOverBudget(t *testing.T) {
	repoRoot := makeTempRepo(t)
	lines := []string{"# Agents", "Read [README.md](README.md)."}
	for index := 0; index < defaultLineBudget; index++ {
		lines = append(lines, "filler")
	}
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), strings.Join(lines, "\n")+"\n")
	writeFile(t, filepath.Join(repoRoot, "README.md"), "# Repo\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Summary.Status != statusDegraded {
		t.Fatalf("summary = %+v", payload.Data)
	}
	if payload.Data.Findings[0].Code != "entrypoint-over-budget" {
		t.Fatalf("finding = %+v", payload.Data.Findings[0])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestEntrypointSyncNotApproved(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"sync", "--json"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Error == nil || payload.Error.Code != "not-approved" {
		t.Fatalf("error = %+v", payload.Error)
	}
}

func decodeResponse(t *testing.T, content []byte) response {
	t.Helper()

	var payload response
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode response: %v\n%s", err, string(content))
	}
	return payload
}

func makeTempRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
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
