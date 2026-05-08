package contextcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/doctor"
)

func TestContextJSONSharesDoctorContract(t *testing.T) {
	repoRoot := makeContextTempRepoRoot(t)
	writeContextFile(t, filepath.Join(repoRoot, "AGENTS.md"), "See README.md.\n")
	writeContextFile(t, filepath.Join(repoRoot, "README.md"), "Anton uses AGENTS.md and anton.yaml.\n")
	writeContextFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	binDir := filepath.Join(repoRoot, "bin")
	codexThreads := filepath.Join(binDir, "codex-threads")
	writeContextFile(t, codexThreads, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(codexThreads, 0o755); err != nil {
		t.Fatalf("chmod codex-threads: %v", err)
	}
	pathValue := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	environ := []string{"PATH=" + pathValue, "HOME=" + repoRoot, "ANTON_TASK_ID=demo_task"}

	var contextStdout bytes.Buffer
	var contextStderr bytes.Buffer
	var doctorStdout bytes.Buffer
	var doctorStderr bytes.Buffer
	exitCode := withContextWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"--json"}, &contextStdout, &contextStderr, environ)
	})
	if exitCode != 0 {
		t.Fatalf("context exit code = %d stdout=%s stderr=%s", exitCode, contextStdout.String(), contextStderr.String())
	}
	exitCode = withContextWorkingDirectory(t, repoRoot, func() int {
		return doctor.Run([]string{"--json"}, &doctorStdout, &doctorStderr, environ)
	})
	if exitCode != 0 {
		t.Fatalf("doctor exit code = %d stdout=%s stderr=%s", exitCode, doctorStdout.String(), doctorStderr.String())
	}

	var contextPayload struct {
		Data struct {
			Contract contract.ContractV1 `json:"contract"`
		} `json:"data"`
	}
	var doctorPayload struct {
		Data struct {
			Contract contract.ContractV1 `json:"contract"`
		} `json:"data"`
	}
	if err := json.Unmarshal(contextStdout.Bytes(), &contextPayload); err != nil {
		t.Fatalf("decode context: %v\n%s", err, contextStdout.String())
	}
	if err := json.Unmarshal(doctorStdout.Bytes(), &doctorPayload); err != nil {
		t.Fatalf("decode doctor: %v\n%s", err, doctorStdout.String())
	}
	if contextPayload.Data.Contract.SchemaVersion != contract.SchemaVersion {
		t.Fatalf("schema_version = %q", contextPayload.Data.Contract.SchemaVersion)
	}
	if !reflect.DeepEqual(contextPayload.Data.Contract, doctorPayload.Data.Contract) {
		t.Fatalf("context contract differs from doctor\ncontext=%#v\ndoctor=%#v", contextPayload.Data.Contract, doctorPayload.Data.Contract)
	}
}

func makeContextTempRepoRoot(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeContextFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	return repoRoot
}

func writeContextFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withContextWorkingDirectory(t *testing.T, path string, fn func() int) int {
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
