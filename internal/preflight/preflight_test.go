package preflight

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestImplementationProfileOKWithTaskIdentity(t *testing.T) {
	root := makePreflightRepo(t)
	writePreflightStatus(t, root, "demo_task")

	payload, exitCode, stderr := runPreflightJSON(t, root, []string{"--profile", "implementation", "--json"}, []string{"ANTON_TASK_ID=demo_task"})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%+v stderr=%s", exitCode, payload, stderr)
	}
	if !payload.OK {
		t.Fatalf("ok = false, want true")
	}
	if payload.Data.Summary.Status != statusOK {
		t.Fatalf("summary status = %q, want ok", payload.Data.Summary.Status)
	}
	if payload.Data.TaskState.Lifecycle != "active" {
		t.Fatalf("task lifecycle = %q", payload.Data.TaskState.Lifecycle)
	}
	if got := checkStatus(payload.Data.Checks, "optional-integrations"); got != statusSkipped {
		t.Fatalf("optional integration status = %q, want skipped", got)
	}
	assertNoPreflightTemps(t, root)
}

func TestMissingTaskIdentityDiffersByProfile(t *testing.T) {
	root := makePreflightRepo(t)

	investigation, investigationExit, investigationStderr := runPreflightJSON(t, root, []string{"--profile", "investigation", "--json"}, nil)
	if investigationExit != 0 {
		t.Fatalf("investigation exit = %d, want 0; stderr=%s", investigationExit, investigationStderr)
	}
	if investigation.Data.Summary.Status != statusWarning {
		t.Fatalf("investigation status = %q, want warning", investigation.Data.Summary.Status)
	}
	if got := checkStatus(investigation.Data.Checks, "task-identity"); got != statusWarning {
		t.Fatalf("investigation task identity status = %q, want warning", got)
	}
	if got := checkStatus(investigation.Data.Checks, "working-directory-writable"); got != statusSkipped {
		t.Fatalf("investigation writable status = %q, want skipped", got)
	}

	implementation, implementationExit, implementationStderr := runPreflightJSON(t, root, []string{"--profile", "implementation", "--json"}, nil)
	if implementationExit != 1 {
		t.Fatalf("implementation exit = %d, want 1; stderr=%s", implementationExit, implementationStderr)
	}
	if implementation.Data.Summary.Status != statusBlocked {
		t.Fatalf("implementation status = %q, want blocked", implementation.Data.Summary.Status)
	}
	if got := checkStatus(implementation.Data.Checks, "task-identity"); got != statusBlocked {
		t.Fatalf("implementation task identity status = %q, want blocked", got)
	}
	assertNoPreflightTemps(t, root)
}

func TestWritableProbeCleansUpAfterBlockedPreflight(t *testing.T) {
	root := makePreflightRepo(t)

	_, exitCode, stderr := runPreflightJSON(t, root, []string{"--profile", "implementation", "--json"}, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s", exitCode, stderr)
	}
	assertNoPreflightTemps(t, root)
}

func TestInvalidConfigReturnsStructuredBlockedJSON(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	writePreflightFile(t, filepath.Join(root, "AGENTS.md"), "# Agent\n")
	writePreflightFile(t, filepath.Join(root, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: repo-root\n"+
		"unexpected: true\n",
	)

	payload, exitCode, stderr := runPreflightJSON(t, root, []string{"--profile", "implementation", "--json"}, []string{"ANTON_TASK_ID=demo_task"})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s", exitCode, stderr)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.Data == nil {
		t.Fatalf("data missing in invalid config response")
	}
	if payload.Data.Summary.Status != statusBlocked {
		t.Fatalf("status = %q, want blocked", payload.Data.Summary.Status)
	}
	if got := checkStatus(payload.Data.Checks, "anton-config"); got != statusBlocked {
		t.Fatalf("config check = %q, want blocked", got)
	}
	if len(payload.Data.Findings) == 0 || payload.Data.Findings[0].Code != "invalid-config" {
		t.Fatalf("findings = %#v, want invalid-config", payload.Data.Findings)
	}
}

type preflightResponse struct {
	OK   bool        `json:"ok"`
	Data *reportData `json:"data"`
}

func runPreflightJSON(t *testing.T, dir string, args []string, environ []string) (preflightResponse, int, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withPreflightWorkingDirectory(t, dir, func() int {
		return Run(args, &stdout, &stderr, environ)
	})
	payload := preflightResponse{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode preflight response: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	return payload, exitCode, stderr.String()
}

func makePreflightRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	initGitRepo(t, root)
	writePreflightFile(t, filepath.Join(root, "AGENTS.md"), "# Agent\n")
	writePreflightFile(t, filepath.Join(root, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: repo-root\n",
	)
	return root
}

func writePreflightStatus(t *testing.T, root string, taskID string) {
	t.Helper()
	writePreflightFile(t, filepath.Join(root, ".anton/tasks/active", taskID, "status.yaml"), ""+
		"version: 1\n"+
		"stable:\n"+
		"  task_id: "+taskID+"\n"+
		"  created_at: 2026-05-13T00:00:00Z\n"+
		"state:\n"+
		"  lifecycle: active\n"+
		"  updated_at: 2026-05-13T00:00:00Z\n"+
		"machine:\n"+
		"  host: test\n"+
		"  execution_target: local\n"+
		"  working_directory: "+root+"\n"+
		"  workspace_kind: git-repo-root\n"+
		"evidence:\n"+
		"  attempts: []\n"+
		"  validations: []\n"+
		"closure:\n"+
		"  finish_state: active\n"+
		"  next_step: continue\n"+
		"  blockers: []\n"+
		"  expected_deliverables: []\n",
	)
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v\n%s", err, string(output))
	}
}

func writePreflightFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func checkStatus(checks []check, name string) string {
	for _, item := range checks {
		if item.Name == name {
			return item.Status
		}
	}
	return ""
}

func assertNoPreflightTemps(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".anton-preflight-*"))
	if err != nil {
		t.Fatalf("glob preflight temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("preflight temp files left behind: %v", matches)
	}
}

func withPreflightWorkingDirectory(t *testing.T, dir string, fn func() int) int {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	return fn()
}
