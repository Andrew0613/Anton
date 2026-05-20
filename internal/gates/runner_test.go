package gates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runstate "github.com/Andrew0613/Anton/internal/run"
)

func TestRunExecutesDeclaredGateJSON(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: helper-pass
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "pass"
    timeout:
      seconds: 5
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "helper-pass"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeRunResponse(t, stdout.Bytes())
	if !payload.OK {
		t.Fatalf("payload should be ok: %#v", payload)
	}
	if payload.Receipt == nil || payload.Receipt.Summary.Passed != 1 || len(payload.Receipt.Results) != 1 {
		t.Fatalf("receipt = %#v", payload.Receipt)
	}
	result := payload.Receipt.Results[0]
	if result.Status != "passed" || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Stdout.Text, "runner helper ok") {
		t.Fatalf("stdout snippet = %#v", result.Stdout)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCapturesFailureExitCode(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: helper-fail
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "fail"
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "helper-fail"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeRunResponse(t, stdout.Bytes())
	result := payload.Receipt.Results[0]
	if result.Status != "failed" || result.ExitCode == nil || *result.ExitCode != 7 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Stderr.Text, "runner helper fail") {
		t.Fatalf("stderr snippet = %#v", result.Stderr)
	}
}

func TestRunDryRunDoesNotExecuteCommand(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker")
	configPath := writeRunnerConfig(t, root, fmt.Sprintf(`version: 1
gates:
  - name: helper-marker
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "marker"
        - %q
`, os.Args[0], marker))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--dry-run", "--config", configPath, "--gate", "helper-marker"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry run should not create marker; stat err=%v", err)
	}
	result := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0]
	if result.Status != "skipped" || result.Reason != "dry-run" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunRejectsShellExecution(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker")
	configPath := writeRunnerConfig(t, root, fmt.Sprintf(`version: 1
gates:
  - name: shell
    type: command
    command:
      argv:
        - sh
        - -c
        - %q
`, "echo unsafe > "+marker))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "shell"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell gate should not create marker; stat err=%v", err)
	}
	result := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0]
	if result.Status != "blocked" || !strings.Contains(result.Reason, "shell execution") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunBlocksCwdEscape(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: cwd-escape
    type: command
    command:
      working_directory: ..
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "pass"
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "cwd-escape"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	result := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0]
	if result.Status != "blocked" || !strings.Contains(result.Reason, "escapes repo root") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunBlocksDestructiveGate(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: destructive
    type: command
    destructive: true
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "pass"
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "destructive"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	result := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0]
	if result.Status != "blocked" || !strings.Contains(result.Reason, "destructive") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunProfileDryRun(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: helper-pass
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "pass"
gate_profiles:
  handoff:
    required:
      - helper-pass
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--dry-run", "--config", configPath, "--profile", "handoff"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	receipt := decodeRunResponse(t, stdout.Bytes()).Receipt
	if receipt.Profile != "handoff" || receipt.Summary.Skipped != 1 || len(receipt.Results) != 1 {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func TestRunAttachRejectsSymlinkReceiptsDir(t *testing.T) {
	repoRoot := t.TempDir()
	writeRunnerFile(t, filepath.Join(repoRoot, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeRunnerFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Entry\n")
	writeRunnerFile(t, filepath.Join(repoRoot, "anton.yaml"), fmt.Sprintf(`version: 1
entrypoint:
  path: AGENTS.md
tasks:
  root: .anton/tasks
  planning_mode: run_manifest
run:
  enabled: true
  manifest: run.json
  receipts_dir: receipts
threads:
  default_project_strategy: repo-root
gates:
  - name: helper-pass
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "pass"
`, os.Args[0]))
	bundleRoot := filepath.Join(repoRoot, ".anton", "tasks", "active", "demo_task")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	writeRunnerFile(t, filepath.Join(bundleRoot, "status.yaml"), ""+
		"version: 1\n"+
		"stable:\n  task_id: demo_task\n  created_at: 2026-05-20T00:00:00Z\n"+
		"state:\n  lifecycle: active\n  updated_at: 2026-05-20T00:00:00Z\n"+
		"machine:\n  host: test\n  execution_target: local\n  working_directory: "+repoRoot+"\n  workspace_kind: git-repo-root\n"+
		"evidence:\n  attempts: []\n  validations: []\n"+
		"closure:\n  finish_state: active\n  next_step: continue\n  blockers: []\n  expected_deliverables: []\n")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	env := []string{"ANTON_TASK_ID=demo_task", "ANTON_RUN_NOW=2026-05-20T00:00:00Z"}
	if code := runstate.Run([]string{"init", "--json"}, io.Discard, io.Discard, env); code != 0 {
		t.Fatalf("run init exit = %d", code)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(bundleRoot, "receipts")); err != nil {
		t.Fatalf("symlink receipts: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--attach-run", "--gate", "helper-pass"}, &stdout, &stderr, env)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout=%s\nstderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "not a symlink") {
		t.Fatalf("expected symlink refusal, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "gates", "helper-pass.json")); !os.IsNotExist(err) {
		t.Fatalf("receipt escaped through symlink; stat err=%v", err)
	}
}

func TestRunCapsOutput(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: helper-spam
    type: command
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "spam"
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "helper-spam"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	snippet := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0].Stdout
	if !snippet.Truncated || snippet.Bytes <= outputCapBytes || len(snippet.Text) != outputCapBytes {
		t.Fatalf("snippet = %#v", snippet)
	}
}

func TestRunTimesOut(t *testing.T) {
	configPath := writeRunnerConfig(t, t.TempDir(), fmt.Sprintf(`version: 1
gates:
  - name: helper-sleep
    type: command
    timeout:
      seconds: 1
    command:
      argv:
        - %q
        - "-test.run=TestGateRunnerHelperProcess"
        - "--"
        - "sleep"
`, os.Args[0]))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"run", "--json", "--config", configPath, "--gate", "helper-sleep"}, &stdout, &stderr, nil)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	result := decodeRunResponse(t, stdout.Bytes()).Receipt.Results[0]
	if result.Status != "timeout" || !strings.Contains(result.Error, "timeout") {
		t.Fatalf("result = %#v", result)
	}
}

func TestGateRunnerHelperProcess(t *testing.T) {
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator == -1 {
		return
	}
	args := os.Args[separator+1:]
	if len(args) == 0 {
		os.Exit(0)
	}
	switch args[0] {
	case "pass":
		fmt.Fprintln(os.Stdout, "runner helper ok")
	case "fail":
		fmt.Fprintln(os.Stderr, "runner helper fail")
		os.Exit(7)
	case "marker":
		if len(args) < 2 {
			os.Exit(2)
		}
		if err := os.WriteFile(args[1], []byte("ran"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "spam":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", outputCapBytes+128))
	case "sleep":
		time.Sleep(2 * time.Second)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func writeRunnerConfig(t *testing.T, root string, content string) string {
	t.Helper()
	path := filepath.Join(root, "anton.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeRunnerFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func decodeRunResponse(t *testing.T, content []byte) gateRunResponse {
	t.Helper()
	payload := gateRunResponse{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode run response: %v\n%s", err, string(content))
	}
	if payload.Receipt == nil {
		t.Fatalf("missing receipt in response: %s", string(content))
	}
	return payload
}
