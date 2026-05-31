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
	if payload.Data == nil || payload.Data.TargetSchema == nil || payload.Data.TargetSchema.Locked {
		t.Fatalf("data = %+v", payload.Data)
	}
	if !strings.Contains(payload.Data.TargetSchema.Reason, "v2 config schema is not locked") {
		t.Fatalf("reason = %q", payload.Data.TargetSchema.Reason)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigratePlanUsesLockedSchemaForReadOnlyTargetPlan(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeConfig(t, repoRoot, ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: .anton/tasks\n"+
		"threads:\n  default_project_strategy: repo-root\n"+
		"migrate:\n"+
		"  target_schema:\n"+
		"    version: 2\n"+
		"    locked: true\n"+
		"  default_target: pkg/target\n")
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
	writeFile(t, filepath.Join(repoRoot, "docs", "move.md"), "Move pkg/target after checking refs.\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"plan", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK || payload.Data == nil || payload.Data.TargetSchema == nil || !payload.Data.TargetSchema.Locked {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Target == nil || payload.Data.Target.Relative != "pkg/target" {
		t.Fatalf("target = %+v", payload.Data.Target)
	}
	if payload.Data.Refs == nil || payload.Data.Refs.Summary.ReferenceHits != 2 {
		t.Fatalf("refs = %+v", payload.Data.Refs)
	}
	if !payload.Data.ReadOnly || len(payload.Data.ProposedChanges) == 0 {
		t.Fatalf("data = %+v", payload.Data)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigrateProjectProgressEmitsMetadataOnlyManifest(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeConfig(t, repoRoot, ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n"+
		"threads:\n  default_project_strategy: repo-root\n"+
		"migrate:\n"+
		"  target_schema:\n"+
		"    version: 2\n"+
		"    locked: true\n"+
		"  default_target: project_progress\n")
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
	writeFile(t, filepath.Join(repoRoot, "docs", "move.md"), "Move project_progress after checking refs.\n")
	writeFile(t, filepath.Join(repoRoot, "project_progress", "Tooling", "tasks", "active", "0001", "status.yaml"), "state: active\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"project-progress", "--metadata-only", "--emit-manifest", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK || payload.Data == nil || payload.Data.Manifest == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Mode != "metadata-only" || !payload.Data.ReadOnly {
		t.Fatalf("data = %+v", payload.Data)
	}
	if payload.Data.Target == nil || payload.Data.Target.Relative != "project_progress" {
		t.Fatalf("target = %+v", payload.Data.Target)
	}
	if payload.Data.Manifest.Inventory.ReferenceHits != 3 {
		t.Fatalf("manifest = %+v", payload.Data.Manifest)
	}
	if len(payload.Data.Manifest.TargetSurfaces) == 0 || len(payload.Data.Manifest.NextSafeActions) == 0 {
		t.Fatalf("manifest = %+v", payload.Data.Manifest)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigrateProjectProgressApplyRequiresApprovalMarker(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"project-progress", "--apply", "--json"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Error == nil || payload.Error.Code != "approval-marker-required" {
		t.Fatalf("error = %+v", payload.Error)
	}
}

func TestMigrateProjectProgressApplyStillNotApprovedWithMarker(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"project-progress", "--apply", "--approval-marker", "marker.txt", "--json"}, &stdout, &stderr, nil)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.Error == nil || payload.Error.Code != "not-approved" {
		t.Fatalf("error = %+v", payload.Error)
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

func TestMigrateReadinessReportsReferencesReadOnly(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeConfig(t, repoRoot, "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")
	writeFile(t, filepath.Join(repoRoot, "docs", "move.md"), "Move pkg/target after checking refs.\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"readiness", "--target", "pkg/target", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if !payload.OK || payload.Data == nil || payload.Data.Refs == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if !payload.Data.ReadOnly || len(payload.Data.ProposedChanges) != 0 {
		t.Fatalf("data = %+v", payload.Data)
	}
	if payload.Data.Recommendation != "go-with-caution" || payload.Data.Refs.Summary.ReferenceHits != 1 {
		t.Fatalf("refs = %+v", payload.Data.Refs)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMigrateReadinessBlocksOutsideRepo(t *testing.T) {
	repoRoot := makeTempRepo(t)
	writeFile(t, filepath.Join(repoRoot, "AGENTS.md"), "# Agents\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDirectory(t, repoRoot, func() int {
		return Run([]string{"readiness", "--target", "../outside", "--json"}, &stdout, &stderr, nil)
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", exitCode, stdout.String())
	}
	payload := decodeResponse(t, stdout.Bytes())
	if payload.OK || payload.Data == nil || payload.Data.Target == nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Data.Recommendation != "no-go" || payload.Data.BlockedReason == "" {
		t.Fatalf("data = %+v", payload.Data)
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

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
