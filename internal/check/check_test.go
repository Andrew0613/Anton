package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Andrew0613/Anton/internal/policy"
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

func TestCheckRunRejectsDeclarationOnlyEntriesRegistry(t *testing.T) {
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
	if exit != 1 {
		t.Fatalf("run exit = %d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var payload struct {
		Data struct {
			Issues []struct {
				Code string `json:"code"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !hasCode(payload.Data.Issues, "policy-rule-kind-missing") {
		t.Fatalf("expected missing check.kind issue: %s", stdout.String())
	}
}

func TestCheckRunEvaluatesStructuredReceiptCoverageAndViewRules(t *testing.T) {
	repo := makeCheckRepo(t)
	writeCheckFile(t, filepath.Join(repo, "docs", "archive", "migrations", "pre_activation_readiness_receipt.md"), "- status: `ready_for_activation`\n")
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checker_coverage.yaml"), ""+
		"coverage:\n"+
		"  - legacy_check_id: A\n"+
		"    status: covered\n"+
		"  - legacy_check_id: B\n"+
		"    status: missing\n")
	writeCheckFile(t, filepath.Join(repo, "docs", "views", "briefs", "repo_health_brief.md"), ""+
		"---\n"+
		"generated_at: \"2026-05-29T20:00:00+08:00\"\n"+
		"source_tree_clean: false\n"+
		"---\n"+
		"# Brief\n")
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"rules:\n"+
		"  - rule_id: readiness-status\n"+
		"    owner: harness\n"+
		"    category: activation\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: file_contains\n"+
		"      path: docs/archive/migrations/pre_activation_readiness_receipt.md\n"+
		"      contains: 'status: `ready_for_activation`'\n"+
		"  - rule_id: checker-coverage\n"+
		"    owner: harness\n"+
		"    category: coverage\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: yaml_all_fields_equal\n"+
		"      path: docs/agent-workflow/registries/checker_coverage.yaml\n"+
		"      field: coverage.*.status\n"+
		"      equals: covered\n"+
		"  - rule_id: view-clean\n"+
		"    owner: harness\n"+
		"    category: views\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: frontmatter_field_equals\n"+
		"      path: docs/views/briefs/repo_health_brief.md\n"+
		"      field: source_tree_clean\n"+
		"      equals: \"true\"\n")

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
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !hasCheckIssue(payload.Data.Issues, "checker-coverage", "yaml-field-mismatch") {
		t.Fatalf("expected checker coverage issue: %s", stdout.String())
	}
	if !hasCheckIssue(payload.Data.Issues, "view-clean", "frontmatter-field-mismatch") {
		t.Fatalf("expected view clean issue: %s", stdout.String())
	}
}

func TestCheckRunRejectsMissingCheckOperands(t *testing.T) {
	repo := makeCheckRepo(t)
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"rules:\n"+
		"  - rule_id: path-empty\n"+
		"    owner: harness\n"+
		"    category: state\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: path_exists\n"+
		"  - rule_id: contains-empty\n"+
		"    owner: harness\n"+
		"    category: state\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: file_contains\n"+
		"      path: AGENTS.md\n"+
		"  - rule_id: field-empty\n"+
		"    owner: harness\n"+
		"    category: views\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: frontmatter_field_present\n"+
		"      path: docs/views/briefs/repo_health_brief.md\n")

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
				Code string `json:"code"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for _, code := range []string{"policy-rule-check-path-missing", "policy-rule-check-contains-missing", "policy-rule-check-field-missing"} {
		if !hasCode(payload.Data.Issues, code) {
			t.Fatalf("expected %s issue: %s", code, stdout.String())
		}
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

func TestCheckRunEvaluatesStateProjectionSourceIntegrity(t *testing.T) {
	repo := makeRealGitCheckRepo(t)
	configureTopicLayerCheckRepo(t, repo)
	statusPath := filepath.Join(repo, "project_progress", "Tooling", "tasks", "active", "0062_hard_cut", "status.yaml")
	cardPath := filepath.Join(repo, "project_progress", "Tooling", "tasks", "active", "0062_hard_cut.md")
	statusContent := "task:\n  id: 0062_hard_cut\n  lifecycle: active\n"
	cardContent := "# 0062 Hard Cut\n"
	writeCheckFile(t, statusPath, statusContent)
	writeCheckFile(t, cardPath, cardContent)
	runCheckGit(t, repo, "add", "AGENTS.md", "anton.yaml", "project_progress")
	runCheckGit(t, repo, "-c", "user.email=anton@example.com", "-c", "user.name=Anton Test", "commit", "-m", "seed legacy state")
	head := strings.TrimSpace(runCheckGit(t, repo, "rev-parse", "HEAD"))

	writeCheckFile(t, filepath.Join(repo, "docs", "state", "tasks", "Tooling", "0062_hard_cut.yaml"), ""+
		"task_id: 0062_hard_cut\n"+
		"topic: Tooling\n"+
		"active: true\n"+
		"lifecycle: active\n"+
		"truth_location: project_progress/Tooling/tasks/active/0062_hard_cut/status.yaml\n"+
		"legacy_card: project_progress/Tooling/tasks/active/0062_hard_cut.md\n"+
		"source_commit: "+head+"\n"+
		"source_status_sha256: 0000000000000000000000000000000000000000000000000000000000000000\n"+
		"source_card_sha256: "+testSHA256(cardContent)+"\n"+
		"workspace:\n"+
		"  path: "+repo+"\n"+
		"  head: 1111111111111111111111111111111111111111\n"+
		"  head_status: live_git_head\n")
	writeCheckFile(t, filepath.Join(repo, "docs", "agent-workflow", "registries", "checks.yaml"), ""+
		"rules:\n"+
		"  - rule_id: state-projection-source-integrity\n"+
		"    owner: harness\n"+
		"    category: state\n"+
		"    severity: error\n"+
		"    check:\n"+
		"      kind: state_projection_source_integrity\n")

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
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !hasCheckIssue(payload.Data.Issues, "state-projection-source-integrity", "state-projection-status-hash-mismatch") {
		t.Fatalf("expected source status hash mismatch: %s", stdout.String())
	}
	if !hasCheckIssue(payload.Data.Issues, "state-projection-source-integrity", "state-projection-workspace-head-stale") {
		t.Fatalf("expected workspace head stale issue: %s", stdout.String())
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

func makeRealGitCheckRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runCheckGit(t, root, "init")
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

func runCheckGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func testSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
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

func hasCode(issues []struct {
	Code string `json:"code"`
}, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests for evaluateFileContainsAll
// ---------------------------------------------------------------------------

func TestEvaluateFileContainsAllAllPresent(t *testing.T) {
	testdata := filepath.Join("testdata", "file_contains_all")
	rule := makeFileContainsAllRule("fca-all-present", filepath.Join(testdata, "present.txt"), []string{"token_one", "token_two"})
	base := "."
	issues := evaluateFileContainsAll(base, rule)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestEvaluateFileContainsAllOneTokenMissing(t *testing.T) {
	testdata := filepath.Join("testdata", "file_contains_all")
	rule := makeFileContainsAllRule("fca-one-missing", filepath.Join(testdata, "missing_token.txt"), []string{"token_one", "token_two"})
	base := "."
	issues := evaluateFileContainsAll(base, rule)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Code != "token-missing" {
		t.Fatalf("expected code token-missing, got %q", issues[0].Code)
	}
}

func TestEvaluateFileContainsAllTwoTokensMissing(t *testing.T) {
	testdata := filepath.Join("testdata", "file_contains_all")
	rule := makeFileContainsAllRule("fca-two-missing", filepath.Join(testdata, "missing_token.txt"), []string{"token_two", "token_three"})
	base := "."
	issues := evaluateFileContainsAll(base, rule)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %v", len(issues), issues)
	}
	for _, issue := range issues {
		if issue.Code != "token-missing" {
			t.Fatalf("expected code token-missing, got %q", issue.Code)
		}
	}
}

func TestEvaluateFileContainsAllFileMissing(t *testing.T) {
	rule := makeFileContainsAllRule("fca-file-missing", "nonexistent/path/file.txt", []string{"token_one"})
	base := "."
	issues := evaluateFileContainsAll(base, rule)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Code != "file-missing" {
		t.Fatalf("expected code file-missing, got %q", issues[0].Code)
	}
}

func makeFileContainsAllRule(ruleID string, path string, tokens []string) policy.Rule {
	return policy.Rule{
		RuleID:   ruleID,
		Owner:    "test",
		Category: "test",
		Severity: "error",
		Blocking: true,
		Check: policy.CheckSpec{
			Kind:   "file_contains_all",
			Path:   path,
			Tokens: tokens,
		},
	}
}

// ---------------------------------------------------------------------------
// Tests for evaluateMarkdownHasSections
// ---------------------------------------------------------------------------

func TestEvaluateMarkdownHasSectionsAllPresent(t *testing.T) {
	testdata := filepath.Join("testdata", "markdown_sections")
	rule := makeMarkdownHasSectionsRule("mhs-all-present", filepath.Join(testdata, "complete.md"), "", []string{"Deliverables", "Execution Contract"})
	base := "."
	issues := evaluateMarkdownHasSections(base, rule)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestEvaluateMarkdownHasSectionsOneMissing(t *testing.T) {
	testdata := filepath.Join("testdata", "markdown_sections")
	rule := makeMarkdownHasSectionsRule("mhs-one-missing", filepath.Join(testdata, "missing_section.md"), "", []string{"Deliverables", "Execution Contract"})
	base := "."
	issues := evaluateMarkdownHasSections(base, rule)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Code != "section-missing" {
		t.Fatalf("expected code section-missing, got %q", issues[0].Code)
	}
}

func TestEvaluateMarkdownHasSectionsPathPatternNoMatch(t *testing.T) {
	rule := makeMarkdownHasSectionsRule("mhs-pattern-nomatch", "", "testdata/markdown_sections/nonexistent_*.md", []string{"Deliverables"})
	base := "."
	issues := evaluateMarkdownHasSections(base, rule)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for no-match pattern, got %d: %v", len(issues), issues)
	}
}

func TestEvaluateMarkdownHasSectionsPathPatternMultiFile(t *testing.T) {
	testdata := filepath.Join("testdata", "markdown_sections")
	// pattern matches both complete.md and missing_section.md
	rule := makeMarkdownHasSectionsRule("mhs-pattern-multi", "", filepath.Join(testdata, "*.md"), []string{"Execution Contract"})
	base := "."
	issues := evaluateMarkdownHasSections(base, rule)
	// complete.md has "Execution Contract", missing_section.md does not → 1 issue
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue from multi-file pattern, got %d: %v", len(issues), issues)
	}
	if issues[0].Code != "section-missing" {
		t.Fatalf("expected code section-missing, got %q", issues[0].Code)
	}
}

func makeMarkdownHasSectionsRule(ruleID string, path string, pathPattern string, sections []string) policy.Rule {
	return policy.Rule{
		RuleID:   ruleID,
		Owner:    "test",
		Category: "test",
		Severity: "error",
		Blocking: true,
		Check: policy.CheckSpec{
			Kind:        "markdown_has_sections",
			Path:        path,
			PathPattern: pathPattern,
			Sections:    sections,
		},
	}
}
