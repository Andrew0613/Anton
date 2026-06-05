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
	if strings.TrimSpace(stdout.String()) != "anton 0.0.5" {
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
	if strings.TrimSpace(stdout.String()) != "anton 0.0.5" {
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
	if !strings.Contains(stdout.String(), `"version": "0.0.5"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestGlobalJSONFlagDispatchesToCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--json", "version"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if !strings.Contains(stdout.String(), `"version": "0.0.5"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSubcommandHelpWorksForPrimarySurfaces(t *testing.T) {
	cases := [][]string{
		{"doctor", "--help"},
		{"context", "--help"},
		{"preflight", "--help"},
		{"task-state", "init", "--help"},
		{"run", "--help"},
		{"history", "show", "--help"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(args, &stdout, &stderr, nil)
			if exitCode != 0 {
				t.Fatalf("exit code = %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Fatalf("stdout missing usage: %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestTaskCommandIsRegistered(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"task", "help"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "anton task resolve") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestGlobalUsageIncludesGatesRun(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--help"}, &stdout, &stderr, nil)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "anton gates <list|check|run> [--json]") {
		t.Fatalf("stdout missing gates run surface: %s", stdout.String())
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

func TestInheritedConfigFeedsGatesAndHistory(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "main")
	worktreeRoot := filepath.Join(root, "wt")
	worktreeGitDir := filepath.Join(mainRoot, ".git", "worktrees", "wt")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree gitdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mainRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir main gitdir: %v", err)
	}
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("mkdir worktree root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainRoot, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write main HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeRoot, ".git"), []byte("gitdir: "+worktreeGitDir+"\n"), 0o644); err != nil {
		t.Fatalf("write worktree .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "HEAD"), []byte("ref: refs/heads/task/demo\n"), 0o644); err != nil {
		t.Fatalf("write worktree HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}
	config := "" +
		"version: 1\n" +
		"entrypoint:\n  path: AGENTS.md\n" +
		"tasks:\n  root: .anton/custom_tasks\n" +
		"threads:\n  default_project_strategy: repo-root\n" +
		"gates:\n" +
		"  - name: inherited-review\n" +
		"    type: command\n" +
		"    required_for: [review]\n" +
		"    command:\n" +
		"      argv: [go, test, ./...]\n" +
		"extensions:\n" +
		"  history:\n" +
		"    work_record_roots:\n" +
		"      - worklog\n"
	if err := os.WriteFile(filepath.Join(mainRoot, "anton.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write main anton.yaml: %v", err)
	}
	writeAppFile(t, filepath.Join(worktreeRoot, ".anton/custom_tasks/active/demo/progress.md"), "# Progress\n\nfrom inherited tasks root\n")
	writeAppFile(t, filepath.Join(worktreeRoot, "worklog/note.md"), "from inherited work root\n")

	withWorkingDirectory(t, worktreeRoot, func() {
		var gatesStdout bytes.Buffer
		var gatesStderr bytes.Buffer
		gatesExit := Run([]string{"gates", "check", "--json"}, &gatesStdout, &gatesStderr, []string{"HOME=" + root})
		if gatesExit != 0 {
			t.Fatalf("gates exit = %d stdout=%s stderr=%s", gatesExit, gatesStdout.String(), gatesStderr.String())
		}
		var gatesPayload struct {
			Data struct {
				Source struct {
					Source string `json:"source"`
				} `json:"source"`
				Summary struct {
					Declared int `json:"declared"`
				} `json:"summary"`
			} `json:"data"`
		}
		if err := json.Unmarshal(gatesStdout.Bytes(), &gatesPayload); err != nil {
			t.Fatalf("decode gates response: %v\n%s", err, gatesStdout.String())
		}
		if gatesPayload.Data.Summary.Declared != 1 {
			t.Fatalf("declared gates = %d, want 1\n%s", gatesPayload.Data.Summary.Declared, gatesStdout.String())
		}
		if gatesPayload.Data.Source.Source != "inherited main-checkout anton.yaml" {
			t.Fatalf("gate source = %q", gatesPayload.Data.Source.Source)
		}

		var historyStdout bytes.Buffer
		var historyStderr bytes.Buffer
		historyExit := Run([]string{"history", "sync", "--json", "--sessions-root", filepath.Join(root, "missing-sessions")}, &historyStdout, &historyStderr, []string{"HOME=" + root})
		if historyExit != 0 {
			t.Fatalf("history exit = %d stdout=%s stderr=%s", historyExit, historyStdout.String(), historyStderr.String())
		}
		var historyPayload struct {
			Data struct {
				Receipts []struct {
					Source struct {
						Path string `json:"path"`
					} `json:"source"`
					Summary string `json:"summary"`
				} `json:"receipts"`
			} `json:"data"`
		}
		if err := json.Unmarshal(historyStdout.Bytes(), &historyPayload); err != nil {
			t.Fatalf("decode history response: %v\n%s", err, historyStdout.String())
		}
		if !historyReceiptsContainPath(historyPayload.Data.Receipts, ".anton/custom_tasks/active/demo/progress.md") {
			t.Fatalf("history did not scan inherited tasks root: %s", historyStdout.String())
		}
		if !historyReceiptsContainPath(historyPayload.Data.Receipts, "worklog/note.md") {
			t.Fatalf("history did not scan inherited work root: %s", historyStdout.String())
		}
	})
}

func historyReceiptsContainPath(receipts []struct {
	Source struct {
		Path string `json:"path"`
	} `json:"source"`
	Summary string `json:"summary"`
}, suffix string) bool {
	for _, receipt := range receipts {
		if strings.HasSuffix(filepath.ToSlash(receipt.Source.Path), filepath.ToSlash(suffix)) {
			return true
		}
	}
	return false
}

func writeAppFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
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
