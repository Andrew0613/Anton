package migrate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigratePlanBlockedUntilV2SchemaLock(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeConfig(t, repoRoot, "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.Data == nil || payload.Data.TargetSchema.Locked {
		t.Fatalf("data = %+v", payload.Data)
	}
	if !strings.Contains(payload.Data.TargetSchema.Reason, "v2 config schema is not locked") {
		t.Fatalf("reason = %q", payload.Data.TargetSchema.Reason)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigratePlanInvalidYAML(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeConfig(t, repoRoot, "version: [\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Error == nil || payload.Error.Code != "migrate-plan-failed" {
		t.Fatalf("error = %+v", payload.Error)
	}
}

func TestMigrateApplyNotApproved(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"apply", "--json"}, &stdout, &stderr, nil)
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
	writeConfig(t, repoRoot, "")
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	return repoRoot
}

func writeConfig(t *testing.T, repoRoot string, content string) {
	t.Helper()

	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if content == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "anton.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write anton.yaml: %v", err)
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
