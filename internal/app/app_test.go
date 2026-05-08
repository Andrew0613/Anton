package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != "anton 0.0.2" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTopLevelVersionFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--version"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != "anton 0.0.2" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestVersionCommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version", "--json"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if !strings.Contains(stdout.String(), `"version": "0.0.2"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTaskAliasNotRegisteredInSliceOne(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"task", "--json"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command: task") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestGatesAndDoctorShareAntonYAMLSchema(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Test Agent\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	config := "" +
		"version: 1\n" +
		"entrypoint:\n  path: AGENTS.md\n" +
		"tasks:\n  root: .anton/tasks\n" +
		"threads:\n  default_project_strategy: repo-root\n" +
		"gates:\n" +
		"  - name: review-smoke\n" +
		"    type: command\n" +
		"    required_for: [review]\n" +
		"    command:\n" +
		"      argv: [go, test, ./...]\n" +
		"extensions:\n" +
		"  history:\n" +
		"    work_record_roots:\n" +
		"      - worklog\n"
	if err := os.WriteFile(filepath.Join(root, "anton.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write anton.yaml: %v", err)
	}

	withWorkingDirectory(t, root, func() {
		var gatesStdout bytes.Buffer
		var gatesStderr bytes.Buffer
		gatesExit := Run([]string{"gates", "check", "--json"}, &gatesStdout, &gatesStderr, []string{"HOME=" + root})
		if gatesExit != 0 {
			t.Fatalf("gates exit = %d stdout=%s stderr=%s", gatesExit, gatesStdout.String(), gatesStderr.String())
		}
		var gatesPayload struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(gatesStdout.Bytes(), &gatesPayload); err != nil {
			t.Fatalf("decode gates response: %v\n%s", err, gatesStdout.String())
		}
		if !gatesPayload.OK {
			t.Fatalf("gates should accept shared config: %s", gatesStdout.String())
		}

		var doctorStdout bytes.Buffer
		var doctorStderr bytes.Buffer
		_ = Run([]string{"doctor", "--json"}, &doctorStdout, &doctorStderr, []string{"HOME=" + root})
		var doctorPayload struct {
			Command string          `json:"command"`
			Data    json.RawMessage `json:"data"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(doctorStdout.Bytes(), &doctorPayload); err != nil {
			t.Fatalf("decode doctor response: %v\nstdout=%s stderr=%s", err, doctorStdout.String(), doctorStderr.String())
		}
		if doctorPayload.Command != "doctor" || len(doctorPayload.Data) == 0 {
			t.Fatalf("doctor did not produce a report: stdout=%s stderr=%s", doctorStdout.String(), doctorStderr.String())
		}
		if doctorPayload.Error != nil && strings.Contains(doctorPayload.Error.Message, "field gates not found") {
			t.Fatalf("doctor rejected shared config schema: %s", doctorPayload.Error.Message)
		}
	})
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
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
			t.Fatalf("restore cwd %s: %v", old, err)
		}
	}()
	fn()
}
