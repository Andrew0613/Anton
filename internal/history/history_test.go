package history

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreAppendReadIdempotent(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	receipt := newReceipt(
		"test",
		Source{Kind: "unit", Path: filepath.Join(root, "source.txt")},
		time.Date(2026, 5, 8, 1, 2, 3, 0, time.UTC),
		"high",
		[]byte("token=secret-value should be hidden"),
		nil,
	)

	appended, warnings := store.AppendNew([]Receipt{receipt})
	if appended != 1 {
		t.Fatalf("first append count = %d, want 1", appended)
	}
	if len(warnings) != 0 {
		t.Fatalf("first append warnings = %#v", warnings)
	}

	appended, warnings = store.AppendNew([]Receipt{receipt})
	if appended != 0 {
		t.Fatalf("second append count = %d, want 0", appended)
	}
	if len(warnings) != 0 {
		t.Fatalf("second append warnings = %#v", warnings)
	}

	result := store.Read()
	if len(result.Receipts) != 1 {
		t.Fatalf("receipt count = %d, want 1", len(result.Receipts))
	}
	if strings.Contains(result.Receipts[0].Summary, "secret-value") {
		t.Fatalf("summary leaked secret: %q", result.Receipts[0].Summary)
	}
}

func TestRunShowEmpty(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root, func() {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run([]string{"show", "--json"}, &stdout, &stderr, []string{"HOME=" + root})
		if exitCode != 0 {
			t.Fatalf("exit code = %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
		}

		var payload response
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v\n%s", err, stdout.String())
		}
		if !payload.OK {
			t.Fatalf("expected ok response")
		}
		if payload.Data == nil || payload.Data.ReceiptCount != 0 {
			t.Fatalf("receipt count = %#v, want 0", payload.Data)
		}
	})
}

func TestRunSyncScansArchiveAndWorkMemoryIdempotently(t *testing.T) {
	root := t.TempDir()
	sessionRoot := filepath.Join(root, "codex", "sessions")
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	sessionPath := filepath.Join(sessionRoot, "rollout.jsonl")
	writeFile(t, sessionPath, `{"timestamp":"2026-05-08T01:02:03Z","type":"user","message":"hello"}`+"\n"+
		`{"timestamp":"2026-05-08T01:03:03Z","type":"assistant","message":"api_key=secret-value"}`+"\n")

	writeFile(t, filepath.Join(root, "anton.yaml"), "version: 1\ntasks:\n  root: .anton/tasks\nextensions:\n  history:\n    work_record_roots:\n      - worklog\n")
	writeFile(t, filepath.Join(root, ".anton/tasks/active/demo/task_plan.md"), "# Plan\n\nShip history.\n")
	writeFile(t, filepath.Join(root, ".anton/tasks/active/demo/progress.md"), "# Progress\n\nDone.\n")
	writeFile(t, filepath.Join(root, ".anton/tasks/active/demo/status.yaml"), "version: 1\nstable:\n  task_id: demo\n")
	writeFile(t, filepath.Join(root, ".anton/memory/events.jsonl"), `{"timestamp":"2026-05-08T02:00:00Z","event":"note"}`+"\n")
	writeFile(t, filepath.Join(root, "worklog/note.md"), "password=hunter2\nkeep bounded.\n")

	withWorkingDirectory(t, root, func() {
		env := []string{"HOME=" + root}
		first := runHistoryJSON(t, []string{"sync", "--json", "--sessions-root", sessionRoot}, env)
		if !first.OK || first.Data == nil {
			t.Fatalf("first sync failed: %#v", first)
		}
		if first.Data.AppendedCount == 0 {
			t.Fatalf("first sync appended no receipts: %#v", first.Data)
		}
		if len(first.Data.Warnings) != 0 {
			t.Fatalf("first sync warnings = %#v", first.Data.Warnings)
		}
		for _, receipt := range first.Data.Receipts {
			if strings.Contains(receipt.Summary, "secret-value") || strings.Contains(receipt.Summary, "hunter2") {
				t.Fatalf("receipt leaked secret: %#v", receipt)
			}
		}

		second := runHistoryJSON(t, []string{"sync", "--json", "--sessions-root", sessionRoot}, env)
		if second.Data == nil || second.Data.AppendedCount != 0 {
			t.Fatalf("second sync appended count = %#v, want 0", second.Data)
		}
		if second.Data.ReceiptCount != first.Data.ReceiptCount {
			t.Fatalf("receipt count changed: got %d want %d", second.Data.ReceiptCount, first.Data.ReceiptCount)
		}
	})
}

func TestRunSyncMissingSessionsWarnsButSucceeds(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root, func() {
		payload := runHistoryJSON(t, []string{"sync", "--json", "--sessions-root", filepath.Join(root, "missing")}, []string{"HOME=" + root})
		if !payload.OK {
			t.Fatalf("sync should succeed with missing sessions: %#v", payload)
		}
		if payload.Data == nil || len(payload.Data.Warnings) == 0 {
			t.Fatalf("expected advisory warning: %#v", payload.Data)
		}
		if payload.Data.Warnings[0].Code != "missing-sessions-root" {
			t.Fatalf("warning code = %q", payload.Data.Warnings[0].Code)
		}
	})
}

func TestRunSyncRefusesSymlinkedReceiptStore(t *testing.T) {
	root := t.TempDir()
	sessionRoot := filepath.Join(root, "codex", "sessions")
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	writeFile(t, filepath.Join(sessionRoot, "rollout.jsonl"), `{"timestamp":"2026-05-08T01:02:03Z","type":"user","message":"hello"}`+"\n")

	historyDir := filepath.Join(root, ".anton", "history")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatalf("mkdir history dir: %v", err)
	}
	outsideFile := filepath.Join(root, "outside-receipts.jsonl")
	if err := os.WriteFile(outsideFile, []byte("sentinel\n"), 0o644); err != nil {
		t.Fatalf("write outside receipt file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(historyDir, "receipts.jsonl")); err != nil {
		t.Fatalf("symlink receipts: %v", err)
	}

	withWorkingDirectory(t, root, func() {
		payload, stdout, stderr := runHistoryJSONWithExit(t, []string{"sync", "--json", "--sessions-root", sessionRoot}, []string{"HOME=" + root}, 1)
		if payload.OK {
			t.Fatalf("sync should refuse symlinked receipt store: stdout=%s stderr=%s", stdout, stderr)
		}
		if payload.Data == nil || !hasWarningCode(payload.Data.Warnings, receiptStoreSymlinkCode) {
			t.Fatalf("expected symlink warning: %#v", payload.Data)
		}
	})

	content, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if string(content) != "sentinel\n" {
		t.Fatalf("outside file was modified: %q", string(content))
	}
}

func TestRunSyncRefusesSymlinkedHistoryDirectory(t *testing.T) {
	root := t.TempDir()
	sessionRoot := filepath.Join(root, "codex", "sessions")
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	writeFile(t, filepath.Join(sessionRoot, "rollout.jsonl"), `{"timestamp":"2026-05-08T01:02:03Z","type":"user","message":"hello"}`+"\n")

	if err := os.MkdirAll(filepath.Join(root, ".anton"), 0o755); err != nil {
		t.Fatalf("mkdir .anton: %v", err)
	}
	outsideDir := filepath.Join(root, "outside-history")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, ".anton", "history")); err != nil {
		t.Fatalf("symlink history dir: %v", err)
	}

	withWorkingDirectory(t, root, func() {
		payload, stdout, stderr := runHistoryJSONWithExit(t, []string{"sync", "--json", "--sessions-root", sessionRoot}, []string{"HOME=" + root}, 1)
		if payload.OK {
			t.Fatalf("sync should refuse symlinked history dir: stdout=%s stderr=%s", stdout, stderr)
		}
		if payload.Data == nil || !hasWarningCode(payload.Data.Warnings, receiptStoreSymlinkCode) {
			t.Fatalf("expected symlink warning: %#v", payload.Data)
		}
	})

	if _, err := os.Stat(filepath.Join(outsideDir, "receipts.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("outside receipt store should not be created, stat err=%v", err)
	}
}

func runHistoryJSON(t *testing.T, args []string, env []string) response {
	t.Helper()
	payload, stdout, stderr := runHistoryJSONWithExit(t, args, env, 0)
	if !payload.OK {
		t.Fatalf("expected ok response: stdout=%s stderr=%s", stdout, stderr)
	}
	return payload
}

func runHistoryJSONWithExit(t *testing.T, args []string, env []string, wantExit int) (response, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(args, &stdout, &stderr, env)
	if exitCode != wantExit {
		t.Fatalf("exit code = %d, want %d stdout=%s stderr=%s", exitCode, wantExit, stdout.String(), stderr.String())
	}
	var payload response
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v\n%s", err, stdout.String())
	}
	return payload, stdout.String(), stderr.String()
}

func hasWarningCode(warnings []Warning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
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
