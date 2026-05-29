package check

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRunAndRepairPlan(t *testing.T) {
	repo := makeCheckRepo(t)
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"rules:\n"+
		"  - rule_id: ensure-state-root\n"+
		"    owner: harness\n"+
		"    category: state\n"+
		"    severity: warning\n"+
		"    autofix: true\n"+
		"    safe_command: mkdir -p docs/state/tasks\n"+
		"    check:\n"+
		"      kind: path_exists\n"+
		"      path: docs/state/tasks\n")

	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	runExit := withCheckWD(t, repo, func() int {
		return Run([]string{"run", "--json"}, &runStdout, &runStderr, nil)
	})
	if runExit != 1 {
		t.Fatalf("run exit = %d stdout=%s stderr=%s", runExit, runStdout.String(), runStderr.String())
	}
	var runPayload struct {
		Data struct {
			Buckets struct {
				SafeAutofixes []struct {
					RuleID string `json:"rule_id"`
				} `json:"safe_autofixes"`
			} `json:"buckets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runStdout.Bytes(), &runPayload); err != nil {
		t.Fatalf("decode run payload: %v", err)
	}
	if len(runPayload.Data.Buckets.SafeAutofixes) != 1 || runPayload.Data.Buckets.SafeAutofixes[0].RuleID != "ensure-state-root" {
		t.Fatalf("unexpected buckets: %s", runStdout.String())
	}

	var repairStdout bytes.Buffer
	var repairStderr bytes.Buffer
	repairExit := withCheckWD(t, repo, func() int {
		return Run([]string{"repair-plan", "--json"}, &repairStdout, &repairStderr, nil)
	})
	if repairExit != 1 {
		t.Fatalf("repair-plan exit = %d stdout=%s stderr=%s", repairExit, repairStdout.String(), repairStderr.String())
	}
	var repairPayload struct {
		Data struct {
			Actions []struct {
				SafeCommand string `json:"safe_command"`
			} `json:"actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(repairStdout.Bytes(), &repairPayload); err != nil {
		t.Fatalf("decode repair payload: %v", err)
	}
	if len(repairPayload.Data.Actions) != 1 {
		t.Fatalf("expected one repair action: %s", repairStdout.String())
	}
}

func TestCheckRunAcceptsProvisionalEntriesRegistry(t *testing.T) {
	repo := makeCheckRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "state", "tasks"), 0o755); err != nil {
		t.Fatalf("mkdir state tasks: %v", err)
	}
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"schema_version: 0\n"+
		"status: provisional_pre_activation\n"+
		"authority: non_authoritative_scaffold\n"+
		"entries:\n"+
		"  - rule_id: CHECK_RULE_PREACTIVATION_SCAFFOLD\n"+
		"    owner: harness\n"+
		"    canonical_source: docs/agent-workflow/registries/checks.yaml\n"+
		"    severity: high\n"+
		"    tier: pre_activation\n"+
		"    planned_anton_surface: anton check run --json\n"+
		"    summary: Registry declaration without executable check.\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := withCheckWD(t, repo, func() int {
		return Run([]string{"run", "--json"}, &stdout, &stderr, nil)
	})
	if exit != 0 {
		t.Fatalf("run exit = %d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var payload struct {
		Data struct {
			Summary struct {
				TotalIssues int `json:"total_issues"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Data.Summary.TotalIssues != 0 {
		t.Fatalf("expected no issues for declaration-only registry: %s", stdout.String())
	}
}

func TestCheckRunEvaluatesStateDualReadParityRule(t *testing.T) {
	repo := makeCheckRepo(t)
	configureTopicLayerCheckRepo(t, repo)
	writeCheckFile(t, filepath.Join(repo, "docs", "state", "tasks", "0062_hard_cut.yaml"), ""+
		"task_id: 0062_hard_cut\n"+
		"topic: Tooling\n"+
		"lifecycle: active\n")
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"rules:\n"+
		"  - rule_id: state-dual-read-parity\n"+
		"    owner: harness\n"+
		"    category: state\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: state_dual_read_parity\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := withCheckWD(t, repo, func() int {
		return Run([]string{"run", "--json"}, &stdout, &stderr, nil)
	})
	if exit != 1 {
		t.Fatalf("run exit = %d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var payload struct {
		Data struct {
			Issues []struct {
				RuleID string `json:"rule_id"`
				Code   string `json:"code"`
			} `json:"issues"`
			Summary struct {
				Blocking int `json:"blocking"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Data.Summary.Blocking == 0 || !hasCheckIssue(payload.Data.Issues, "state-dual-read-parity", "state-dual-read-missing-current-legacy") {
		t.Fatalf("expected state dual-read parity issue: %s", stdout.String())
	}
}

func makeCheckRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCheckFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeCheckFile(t, filepath.Join(root, "AGENTS.md"), "# Agents\n")
	writeCheckFile(t, filepath.Join(root, "anton.yaml"), "version: 1\nentrypoint:\n  path: AGENTS.md\ntasks:\n  root: .anton/tasks\nthreads:\n  default_project_strategy: repo-root\n")
	return root
}

func configureTopicLayerCheckRepo(t *testing.T, repo string) {
	t.Helper()
	writeCheckFile(t, filepath.Join(repo, "anton.yaml"), ""+
		"version: 1\n"+
		"entrypoint:\n  path: AGENTS.md\n"+
		"tasks:\n  root: project_progress\n  layout: topic-layer\n  status_schema: physedit-v1\n"+
		"threads:\n  default_project_strategy: repo-root\n")
}

func writeCheckFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func withCheckWD(t *testing.T, dir string, fn func() int) int {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
	return fn()
}

func hasCheckIssue(issues []struct {
	RuleID string `json:"rule_id"`
	Code   string `json:"code"`
}, ruleID string, code string) bool {
	for _, issue := range issues {
		if issue.RuleID == ruleID && issue.Code == code {
			return true
		}
	}
	return false
}
