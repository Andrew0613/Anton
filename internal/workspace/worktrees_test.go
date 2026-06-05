package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- porcelain parser tests ---

func TestParseWorktreePorcelainSingleMain(t *testing.T) {
	input := `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

`
	entries := parseWorktreePorcelain(input)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.path != filepath.Clean("/home/user/repo") {
		t.Errorf("path = %q", e.path)
	}
	if e.head != "abc123" {
		t.Errorf("head = %q", e.head)
	}
	if e.branch != "main" {
		t.Errorf("branch = %q", e.branch)
	}
	if e.bare {
		t.Errorf("bare should be false")
	}
}

func TestParseWorktreePorcelainMultipleEntries(t *testing.T) {
	input := `worktree /home/user/repo
HEAD aaa111
branch refs/heads/main

worktree /home/user/wt-feature
HEAD bbb222
branch refs/heads/feature-x

worktree /home/user/wt-bare
HEAD ccc333
bare

`
	entries := parseWorktreePorcelain(input)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Main entry (index 0)
	if entries[0].branch != "main" {
		t.Errorf("entries[0].branch = %q, want main", entries[0].branch)
	}

	// Feature branch (index 1)
	if entries[1].path != filepath.Clean("/home/user/wt-feature") {
		t.Errorf("entries[1].path = %q", entries[1].path)
	}
	if entries[1].branch != "feature-x" {
		t.Errorf("entries[1].branch = %q, want feature-x", entries[1].branch)
	}
	if entries[1].bare {
		t.Errorf("entries[1].bare should be false")
	}

	// Bare entry (index 2)
	if !entries[2].bare {
		t.Errorf("entries[2].bare should be true")
	}
	if entries[2].branch != "" {
		t.Errorf("entries[2].branch = %q, want empty", entries[2].branch)
	}
}

func TestParseWorktreePorcelainDetachedHead(t *testing.T) {
	input := `worktree /home/user/repo
HEAD aaa111
branch refs/heads/main

worktree /home/user/wt-detached
HEAD ddd444

`
	entries := parseWorktreePorcelain(input)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].branch != "" {
		t.Errorf("detached entry branch = %q, want empty", entries[1].branch)
	}
	if entries[1].head != "ddd444" {
		t.Errorf("detached entry head = %q, want ddd444", entries[1].head)
	}
}

func TestParseWorktreePorcelainEmptyInput(t *testing.T) {
	entries := parseWorktreePorcelain("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(entries))
	}
}

func TestParseWorktreePorcelainNoTrailingNewline(t *testing.T) {
	// Some git versions omit the trailing blank line on the last block.
	input := `worktree /home/user/repo
HEAD aaa111
branch refs/heads/main

worktree /home/user/wt2
HEAD bbb222
branch refs/heads/task-1`

	entries := parseWorktreePorcelain(input)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[1].branch != "task-1" {
		t.Errorf("entries[1].branch = %q, want task-1", entries[1].branch)
	}
}

// --- artifact detection tests ---

func TestBuildWorktreeStatusDetectsArtifacts(t *testing.T) {
	dir := t.TempDir()
	// Create a fake results/ dir to trigger HasArtifacts.
	if err := os.MkdirAll(filepath.Join(dir, "results"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	status, _ := buildWorktreeStatus(dir, "", "", false)
	if !status.HasArtifacts {
		t.Errorf("HasArtifacts = false, want true")
	}
	if status.SafeToRemove {
		t.Errorf("SafeToRemove = true, want false (artifacts present)")
	}
	if status.Blocker == "" {
		t.Errorf("Blocker should not be empty when artifacts present")
	}
}

func TestBuildWorktreeStatusNoArtifacts(t *testing.T) {
	dir := t.TempDir()
	// No artifact directories.

	status, _ := buildWorktreeStatus(dir, "", "", false)
	if status.HasArtifacts {
		t.Errorf("HasArtifacts = true, want false")
	}
}

// --- Run dispatch test ---

func TestWorkspaceWorktreesUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"worktrees", "bogus"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if stderr.Len() == 0 {
		t.Errorf("expected stderr output for unknown subcommand")
	}
}

func TestWorkspaceWorktreesNoSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"worktrees"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}

func TestWorkspaceWorktreesInspectMissingArg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"worktrees", "inspect"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}

func TestWorkspaceWorktreesInspectValidDir(t *testing.T) {
	// inspectWorktree on an arbitrary dir should not crash even if git isn't present.
	dir := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"worktrees", "inspect", dir}, &stdout, &stderr, nil)
	// We expect success (0) since the function is best-effort.
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s", exitCode, stdout.String(), stderr.String())
	}

	var payload worktreesResponse
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v\n%s", err, stdout.String())
	}
	if !payload.OK {
		t.Errorf("ok = false, want true")
	}
	if payload.Single == nil {
		t.Errorf("single = nil, want non-nil")
	}
}

func TestWorkspaceWorktreesRemoveMissingArg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"worktrees", "remove"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}
