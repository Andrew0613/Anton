package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceInspectEmpty(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"inspect", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK || payload.Data == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.Data.Roots) != 0 {
		t.Fatalf("roots = %+v, want empty", payload.Data.Roots)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkspaceInspectSuccess(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/workspaces")
	writeFile(t, filepath.Join(repoRoot, ".anton", "workspaces", "ISSUE-42", ".keep"), "fixture\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"inspect", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Summary.ProjectCount != 1 {
		t.Fatalf("summary = %+v", payload.Data)
	}
	if payload.Data.Roots[0].Projects[0].Name != "ISSUE-42" {
		t.Fatalf("projects = %+v", payload.Data.Roots[0].Projects)
	}
}

func TestWorkspaceCheckDoesNotTreatSiblingPrefixAsChild(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/workspaces/foo")
	writeFile(t, filepath.Join(repoRoot, ".anton", "workspaces", "foo", "PROJECT", ".keep"), "fixture\n")
	writeFile(t, filepath.Join(repoRoot, ".anton", "workspaces", "foobar", ".keep"), "sibling\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Summary.ProjectCount != 1 {
		t.Fatalf("summary = %+v", payload.Data)
	}
	if payload.Data.Roots[0].Projects[0].Name != "PROJECT" {
		t.Fatalf("projects = %+v", payload.Data.Roots[0].Projects)
	}
}

func TestWorkspaceCheckBlocksSymlinkEscape(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/workspaces")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".anton", "workspaces"), 0o755); err != nil {
		t.Fatalf("mkdir workspaces: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repoRoot, ".anton", "workspaces", "ESCAPE")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.Data == nil || payload.Data.Findings[0].Code != "workspace-project-symlink-escape" {
		t.Fatalf("findings = %+v", payload.Data)
	}
}

func TestWorkspaceCheckBlocksTraversalRoot(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/../outside")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"check", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Findings[0].Code != "workspace-root-traversal" {
		t.Fatalf("findings = %+v", payload.Data)
	}
}

func TestWorkspacePrepareNotApproved(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"prepare", "--json"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Error == nil || payload.Error.Code != "not-approved" {
		t.Fatalf("error = %+v", payload.Error)
	}
}

func TestWorkspaceCleanupPlanClassifiesResultPersistence(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/workspaces")
	writeFile(t, filepath.Join(repoRoot, "results", "evidence.txt"), "preserve me\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"cleanup-plan", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Cockpit == nil || len(payload.Data.Cockpit.Workspaces) == 0 {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Cockpit.Workspaces[0].Classification != "result_persist_required" {
		t.Fatalf("classification = %s payload=%s", payload.Data.Cockpit.Workspaces[0].Classification, stdout.String())
	}
}

func TestWorkspaceRefsFindsTextReferencesAndReportsSkippedRoots(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeWorkspaceConfig(t, repoRoot, ".anton/workspaces")
	writeFile(t, filepath.Join(repoRoot, "docs", "move.md"), "Move pkg/target before closeout.\n")
	writeFile(t, filepath.Join(repoRoot, "config", "paths.yaml"), "target: pkg/target\n")
	writeFile(t, filepath.Join(repoRoot, "internal", "paths.go"), "package paths\n\nconst target = \"pkg/target\"\n")
	writeFile(t, filepath.Join(repoRoot, "scripts", "move.sh"), "#!/bin/sh\nprintf '%s\\n' pkg/target\n")
	writeFile(t, filepath.Join(repoRoot, "vendor", "ignored.md"), "pkg/target\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"refs", "--target", "pkg/target", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK || payload.Data == nil || payload.Data.Refs == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Refs.Summary.ReferenceHits != 4 {
		t.Fatalf("reference hits = %+v", payload.Data.Refs.ReferenceHits)
	}
	kinds := map[string]bool{}
	for _, hit := range payload.Data.Refs.ReferenceHits {
		kinds[hit.Kind] = true
	}
	for _, kind := range []string{"markdown", "yaml", "go", "shell"} {
		if !kinds[kind] {
			t.Fatalf("missing %s hit in %+v", kind, payload.Data.Refs.ReferenceHits)
		}
	}
	foundVendor := false
	for _, skipped := range payload.Data.Refs.SkippedRoots {
		if skipped.Path == "vendor" {
			foundVendor = true
		}
	}
	if !foundVendor {
		t.Fatalf("skipped roots = %+v", payload.Data.Refs.SkippedRoots)
	}
	if payload.Data.Refs.Summary.Recommendation != "go-with-caution" {
		t.Fatalf("summary = %+v", payload.Data.Refs.Summary)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkspaceRefsBlocksTargetOutsideRepo(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"refs", "--target", "../outside", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.OK || payload.Data == nil || payload.Data.Refs == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Refs.Findings[0].Code != "target-escapes-repo" {
		t.Fatalf("findings = %+v", payload.Data.Refs.Findings)
	}
}

func TestWorkspaceRefsBlocksSymlinkEscape(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repoRoot, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"refs", "--target", "escape/file.txt", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Refs == nil || payload.Data.Refs.Findings[0].Code != "target-symlink-escape" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestWorkspaceRefsReportsWorktreeAndTaskRootOverlap(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
	target := filepath.Join(repoRoot, ".anton", "tasks", "active", "ISSUE-1")
	writeFile(t, filepath.Join(target, "status.yaml"), "status: active\n")
	worktreeMeta := filepath.Join(repoRoot, ".git", "worktrees", "ISSUE-1")
	writeFile(t, filepath.Join(worktreeMeta, "HEAD"), "ref: refs/heads/task/ISSUE-1\n")
	writeFile(t, filepath.Join(worktreeMeta, "gitdir"), filepath.Join(target, ".git")+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"refs", "--target", ".anton/tasks/active/ISSUE-1", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Data == nil || payload.Data.Refs == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if !payload.Data.Refs.TaskBundle.TargetOverlaps || payload.Data.Refs.TaskBundle.State != "active" {
		t.Fatalf("task bundle = %+v", payload.Data.Refs.TaskBundle)
	}
	foundOverlap := false
	for _, worktree := range payload.Data.Refs.Worktrees {
		if !worktree.Current && worktree.OverlapsTarget && worktree.Branch == "task/ISSUE-1" {
			foundOverlap = true
		}
	}
	if !foundOverlap {
		t.Fatalf("worktrees = %+v", payload.Data.Refs.Worktrees)
	}
}

func writeWorkspaceConfig(t *testing.T, repoRoot string, root string) {
	t.Helper()
	writeFile(t, filepath.Join(repoRoot, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n  workspace_roots:\n    - "+root+"\n")
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
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
