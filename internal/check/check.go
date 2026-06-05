package check

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/policy"
	"github.com/Andrew0613/Anton/internal/state"
	"gopkg.in/yaml.v3"
)

type options struct {
	JSON    bool
	Profile string
}

type ActionableIssue struct {
	RuleID          string `json:"rule_id"`
	Owner           string `json:"owner"`
	Category        string `json:"category"`
	Severity        string `json:"severity"`
	CanonicalSource string `json:"canonical_source,omitempty"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	Blocking        bool   `json:"blocking"`
	Autofix         bool   `json:"autofix"`
	SafeCommand     string `json:"safe_command,omitempty"`
	Bucket          string `json:"bucket"`
}

type checkBuckets struct {
	BlockingNow          []ActionableIssue `json:"blocking_now"`
	SafeAutofixes        []ActionableIssue `json:"safe_autofixes"`
	HumanDecisionsNeeded []ActionableIssue `json:"human_decisions_needed"`
	ArchiveOnly          []ActionableIssue `json:"archive_or_history_only"`
}

type summary struct {
	Status      string `json:"status"`
	TotalIssues int    `json:"total_issues"`
	Blocking    int    `json:"blocking"`
	Autofixes   int    `json:"autofixes"`
}

type runData struct {
	Adapter          string            `json:"adapter"`
	WorkingDirectory string            `json:"working_directory"`
	RepositoryRoot   string            `json:"repository_root,omitempty"`
	PolicyRoot       string            `json:"policy_root"`
	PolicySource     string            `json:"policy_source,omitempty"`
	StateRoot        string            `json:"state_root"`
	Issues           []ActionableIssue `json:"issues"`
	Buckets          checkBuckets      `json:"buckets"`
	Summary          summary           `json:"summary"`
}

type repairAction struct {
	RuleID      string `json:"rule_id"`
	SafeCommand string `json:"safe_command"`
	Reason      string `json:"reason"`
}

type repairPlanData struct {
	Adapter          string         `json:"adapter"`
	WorkingDirectory string         `json:"working_directory"`
	RepositoryRoot   string         `json:"repository_root,omitempty"`
	PolicyRoot       string         `json:"policy_root"`
	Actions          []repairAction `json:"actions"`
	ManualFollowUps  []string       `json:"manual_follow_ups"`
	Summary          summary        `json:"summary"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    any           `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}
	switch args[0] {
	case "run":
		return runCheck(args[1:], stdout, stderr, environ)
	case "repair-plan":
		return runRepairPlan(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown check command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runCheck(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("check run", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	report, err := collect(environ, opts.Profile)
	if err != nil {
		return writeError("check run", "check-run-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	exitCode := 0
	if report.Summary.Status != "ok" {
		exitCode = 1
	}
	return writeResponse("check run", report, opts.JSON, stdout, exitCode)
}

func runRepairPlan(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("check repair-plan", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	report, err := collect(environ, opts.Profile)
	if err != nil {
		return writeError("check repair-plan", "check-repair-plan-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	actions := []repairAction{}
	manual := []string{}
	for _, issue := range report.Buckets.SafeAutofixes {
		if strings.TrimSpace(issue.SafeCommand) == "" {
			continue
		}
		actions = append(actions, repairAction{
			RuleID:      issue.RuleID,
			SafeCommand: issue.SafeCommand,
			Reason:      issue.Message,
		})
	}
	for _, issue := range report.Buckets.BlockingNow {
		manual = append(manual, fmt.Sprintf("%s: %s", issue.RuleID, issue.Message))
	}
	for _, issue := range report.Buckets.HumanDecisionsNeeded {
		manual = append(manual, fmt.Sprintf("%s: %s", issue.RuleID, issue.Message))
	}
	data := repairPlanData{
		Adapter:          report.Adapter,
		WorkingDirectory: report.WorkingDirectory,
		RepositoryRoot:   report.RepositoryRoot,
		PolicyRoot:       report.PolicyRoot,
		Actions:          actions,
		ManualFollowUps:  manual,
		Summary:          report.Summary,
	}
	exitCode := 0
	if report.Summary.Status != "ok" {
		exitCode = 1
	}
	return writeResponse("check repair-plan", data, opts.JSON, stdout, exitCode)
}

func collect(environ []string, profile string) (runData, error) {
	wd, err := os.Getwd()
	if err != nil {
		return runData{}, err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return runData{}, err
	}
	registry, err := policy.Load(resolved)
	if err != nil {
		return runData{}, err
	}
	inventory, err := state.LoadInventory(resolved, false)
	if err != nil {
		return runData{}, err
	}

	issues := []ActionableIssue{}
	for _, item := range registry.Issues {
		issues = append(issues, issueFromPolicy(item))
	}
	for _, item := range inventory.Issues {
		issues = append(issues, issueFromState(item))
	}

	base := resolved.Context.WorkingDirectory
	if resolved.Context.RepositoryRoot != "" {
		base = resolved.Context.RepositoryRoot
	}
	for _, rule := range filterRulesByProfile(registry.Rules, profile) {
		if strings.TrimSpace(rule.Check.Kind) == "" {
			continue
		}
		if rule.Check.Kind == "state_dual_read_parity" {
			dualReadInventory, loadErr := state.LoadInventory(resolved, true)
			if loadErr != nil {
				return runData{}, loadErr
			}
			for _, item := range dualReadInventory.Issues {
				if !isDualReadStateIssue(item) {
					continue
				}
				issues = append(issues, issueFromDualReadRule(rule, item))
			}
			continue
		}
		if rule.Check.Kind == "state_projection_source_integrity" {
			issues = append(issues, evaluateStateProjectionSourceIntegrity(base, rule, inventory.Tasks)...)
			continue
		}
		if rule.Check.Kind == "file_contains_all" {
			issues = append(issues, evaluateFileContainsAll(base, rule)...)
			continue
		}
		if rule.Check.Kind == "markdown_has_sections" {
			issues = append(issues, evaluateMarkdownHasSections(base, rule)...)
			continue
		}
		evaluated, failed := evaluateRule(base, rule)
		if failed {
			issues = append(issues, evaluated)
		}
	}

	buckets := bucketize(issues)
	report := runData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: resolved.Context.WorkingDirectory,
		RepositoryRoot:   resolved.Context.RepositoryRoot,
		PolicyRoot:       registry.Root,
		PolicySource:     registry.SourceFile,
		StateRoot:        inventory.StateRoot,
		Issues:           issues,
		Buckets:          buckets,
		Summary: summary{
			Status:      summarizeStatus(buckets),
			TotalIssues: len(issues),
			Blocking:    len(buckets.BlockingNow),
			Autofixes:   len(buckets.SafeAutofixes),
		},
	}
	return report, nil
}

func filterRulesByProfile(rules []policy.Rule, profile string) []policy.Rule {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return rules
	}
	selected := make([]policy.Rule, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.Category) == profile {
			selected = append(selected, rule)
		}
	}
	return selected
}

func evaluateRule(base string, rule policy.Rule) (ActionableIssue, bool) {
	issue := actionableIssueFromRule(rule)
	target := filepath.Clean(filepath.Join(base, rule.Check.Path))
	switch rule.Check.Kind {
	case "path_exists":
		if statPath(target) {
			return issue, false
		}
		issue.Code = "path-missing"
		issue.Message = fmt.Sprintf("required path is missing: %s", rule.Check.Path)
	case "path_missing":
		if !statPath(target) {
			return issue, false
		}
		issue.Code = "path-should-be-missing"
		issue.Message = fmt.Sprintf("path must be absent: %s", rule.Check.Path)
	case "file_contains":
		content, err := os.ReadFile(target)
		if err == nil && strings.Contains(string(content), rule.Check.Contains) {
			return issue, false
		}
		issue.Code = "file-content-mismatch"
		issue.Message = fmt.Sprintf("file %s does not contain required token", rule.Check.Path)
	case "yaml_field_equals":
		values, err := yamlFieldValues(target, rule.Check.Field, false)
		if err == nil && len(values) == 1 && values[0] == rule.Check.Equals {
			return issue, false
		}
		issue.Code = "yaml-field-mismatch"
		issue.Message = fmt.Sprintf("YAML field %s in %s does not equal %q", rule.Check.Field, rule.Check.Path, rule.Check.Equals)
	case "yaml_all_fields_equal":
		values, err := yamlFieldValues(target, rule.Check.Field, true)
		if err == nil && len(values) > 0 && allEqual(values, rule.Check.Equals) {
			return issue, false
		}
		issue.Code = "yaml-field-mismatch"
		issue.Message = fmt.Sprintf("not all YAML field values %s in %s equal %q", rule.Check.Field, rule.Check.Path, rule.Check.Equals)
	case "frontmatter_field_present":
		values, err := frontmatterFieldValues(target, rule.Check.Field)
		if err == nil && len(values) == 1 && strings.TrimSpace(values[0]) != "" {
			return issue, false
		}
		issue.Code = "frontmatter-field-missing"
		issue.Message = fmt.Sprintf("frontmatter field %s in %s is missing or empty", rule.Check.Field, rule.Check.Path)
	case "frontmatter_field_equals":
		values, err := frontmatterFieldValues(target, rule.Check.Field)
		if err == nil && len(values) == 1 && values[0] == rule.Check.Equals {
			return issue, false
		}
		issue.Code = "frontmatter-field-mismatch"
		issue.Message = fmt.Sprintf("frontmatter field %s in %s does not equal %q", rule.Check.Field, rule.Check.Path, rule.Check.Equals)
	default:
		issue.Code = "unsupported-rule-kind"
		issue.Message = fmt.Sprintf("unsupported policy check.kind %q", rule.Check.Kind)
		issue.Blocking = true
		issue.Autofix = false
	}
	issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
	return issue, true
}

func evaluateFileContainsAll(base string, rule policy.Rule) []ActionableIssue {
	target := filepath.Join(base, rule.Check.Path)
	content, err := os.ReadFile(target)
	if err != nil {
		issue := actionableIssueFromRule(rule)
		issue.Code = "file-missing"
		issue.Message = fmt.Sprintf("file %s does not exist or cannot be read", rule.Check.Path)
		issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
		return []ActionableIssue{issue}
	}
	text := string(content)
	var issues []ActionableIssue
	for _, token := range rule.Check.Tokens {
		if !strings.Contains(text, token) {
			issue := actionableIssueFromRule(rule)
			issue.Code = "token-missing"
			issue.Message = fmt.Sprintf("file %s does not contain required token: %q", rule.Check.Path, token)
			issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
			issues = append(issues, issue)
		}
	}
	return issues
}

func evaluateMarkdownHasSections(base string, rule policy.Rule) []ActionableIssue {
	var paths []string
	if rule.Check.PathPattern != "" {
		pattern := filepath.Join(base, rule.Check.PathPattern)
		matches, err := filepath.Glob(pattern)
		if err == nil {
			paths = matches
		}
	} else if rule.Check.Path != "" {
		paths = []string{filepath.Join(base, rule.Check.Path)}
	}

	var issues []ActionableIssue
	for _, filePath := range paths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // skip unreadable files
		}
		// Parse H2 headings (lines starting with "## ")
		found := map[string]bool{}
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") {
				heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
				found[heading] = true
			}
		}
		// Compute relative path for message
		rel := filePath
		if r, err := filepath.Rel(base, filePath); err == nil {
			rel = r
		}
		for _, section := range rule.Check.Sections {
			if !found[section] {
				issue := actionableIssueFromRule(rule)
				issue.Code = "section-missing"
				issue.Message = fmt.Sprintf("file %s is missing required section: %q", rel, section)
				issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
				issues = append(issues, issue)
			}
		}
	}
	return issues
}

func evaluateStateProjectionSourceIntegrity(base string, rule policy.Rule, tasks []state.TaskRecord) []ActionableIssue {
	issues := []ActionableIssue{}
	for _, task := range tasks {
		source := taskSourceFields(task)
		if source.Commit == "" {
			issues = append(issues, taskProjectionIssue(rule, task, "state-projection-source-commit-missing", fmt.Sprintf("task %s projection has no source_commit", task.TaskID)))
		}
		if task.TruthLocation != "" || source.StatusSHA256 != "" {
			issues = append(issues, compareProjectedSourceHash(base, rule, task, source.Commit, task.TruthLocation, source.StatusSHA256, "status")...)
		}
		if task.LegacyCard != "" || source.CardSHA256 != "" {
			issues = append(issues, compareProjectedSourceHash(base, rule, task, source.Commit, task.LegacyCard, source.CardSHA256, "card")...)
		}
		if strings.TrimSpace(task.Workspace.Path) != "" {
			issues = append(issues, compareProjectedWorkspaceHead(rule, task)...)
		}
	}
	return issues
}

type taskSource struct {
	Commit       string
	StatusSHA256 string
	CardSHA256   string
}

func taskSourceFields(task state.TaskRecord) taskSource {
	source := taskSource{
		Commit:       strings.TrimSpace(task.SourceCommit),
		StatusSHA256: strings.TrimSpace(task.SourceStatusSHA256),
		CardSHA256:   strings.TrimSpace(task.SourceCardSHA256),
	}
	for _, part := range strings.Split(task.SourceRevision, ";") {
		part = strings.TrimSpace(part)
		switch {
		case source.Commit == "" && strings.HasPrefix(part, "git:"):
			source.Commit = strings.TrimSpace(strings.TrimPrefix(part, "git:"))
		case source.StatusSHA256 == "" && strings.HasPrefix(part, "status_sha256:"):
			source.StatusSHA256 = strings.TrimSpace(strings.TrimPrefix(part, "status_sha256:"))
		case source.CardSHA256 == "" && strings.HasPrefix(part, "card_sha256:"):
			source.CardSHA256 = strings.TrimSpace(strings.TrimPrefix(part, "card_sha256:"))
		}
	}
	return source
}

func compareProjectedSourceHash(base string, rule policy.Rule, task state.TaskRecord, commit string, path string, expected string, label string) []ActionableIssue {
	if strings.TrimSpace(path) == "" {
		return []ActionableIssue{taskProjectionIssue(rule, task, fmt.Sprintf("state-projection-%s-path-missing", label), fmt.Sprintf("task %s projection has %s hash but no %s path", task.TaskID, label, label))}
	}
	if strings.TrimSpace(expected) == "" {
		return []ActionableIssue{taskProjectionIssue(rule, task, fmt.Sprintf("state-projection-%s-hash-missing", label), fmt.Sprintf("task %s projection has %s path but no %s SHA256", task.TaskID, label, label))}
	}
	if strings.TrimSpace(commit) == "" {
		return nil
	}
	actual, err := gitBlobSHA256(base, commit, path)
	if err != nil {
		return []ActionableIssue{taskProjectionIssue(rule, task, fmt.Sprintf("state-projection-%s-source-unreadable", label), fmt.Sprintf("task %s projection cannot read %s %s at %s: %v", task.TaskID, label, path, commit, err))}
	}
	if actual != strings.TrimSpace(expected) {
		return []ActionableIssue{taskProjectionIssue(rule, task, fmt.Sprintf("state-projection-%s-hash-mismatch", label), fmt.Sprintf("task %s projection %s SHA256 does not match %s at %s", task.TaskID, label, path, commit))}
	}
	return nil
}

func compareProjectedWorkspaceHead(rule policy.Rule, task state.TaskRecord) []ActionableIssue {
	liveHead, liveStatus := liveGitHead(task.Workspace.Path)
	projectedHead := strings.TrimSpace(task.Workspace.Head)
	headStatus := strings.TrimSpace(task.Workspace.HeadStatus)
	if liveHead != "" {
		if headStatus != "" && headStatus != "live_git_head" {
			return nil
		}
		if projectedHead == "" {
			return []ActionableIssue{taskProjectionIssue(rule, task, "state-projection-workspace-head-missing-live", fmt.Sprintf("task %s projection omits workspace.head for live checkout %s", task.TaskID, task.Workspace.Path))}
		}
		if projectedHead != liveHead {
			return []ActionableIssue{taskProjectionIssue(rule, task, "state-projection-workspace-head-stale", fmt.Sprintf("task %s projection workspace.head %s does not match live HEAD %s at %s", task.TaskID, projectedHead, liveHead, task.Workspace.Path))}
		}
		return nil
	}
	if headStatus != "" && headStatus != "live_git_head" {
		return nil
	}
	if projectedHead != "" {
		return []ActionableIssue{taskProjectionIssue(rule, task, "state-projection-workspace-head-nonlive", fmt.Sprintf("task %s projection has workspace.head for non-live workspace %s (%s)", task.TaskID, task.Workspace.Path, liveStatus))}
	}
	if headStatus == "" || headStatus == "live_git_head" {
		return []ActionableIssue{taskProjectionIssue(rule, task, "state-projection-workspace-head-status-missing", fmt.Sprintf("task %s projection does not explain non-live workspace %s (%s)", task.TaskID, task.Workspace.Path, liveStatus))}
	}
	return nil
}

func taskProjectionIssue(rule policy.Rule, task state.TaskRecord, code string, message string) ActionableIssue {
	issue := actionableIssueFromRule(rule)
	issue.Code = code
	issue.Message = message
	issue.CanonicalSource = task.SourceFile
	issue.Bucket = chooseBucket(issue, false)
	return issue
}

func gitBlobSHA256(base string, commit string, path string) (string, error) {
	relative, err := repoRelativePath(base, path)
	if err != nil {
		return "", err
	}
	output, err := gitOutput(base, 3*time.Second, "show", fmt.Sprintf("%s:%s", commit, filepath.ToSlash(relative)))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(output))
	return fmt.Sprintf("%x", sum[:]), nil
}

func repoRelativePath(base string, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return "", err
		}
		path = relative
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("path %q is outside repository root", path)
	}
	return cleaned, nil
}

func liveGitHead(path string) (string, string) {
	if statDir(path) {
		output, err := gitOutput(path, 2*time.Second, "rev-parse", "HEAD")
		if err == nil && strings.TrimSpace(output) != "" {
			return strings.TrimSpace(output), "live_git_head"
		}
		if err != nil && strings.Contains(err.Error(), "timeout") {
			return "", "head_probe_timeout"
		}
		return "", "not_git_checkout"
	}
	return "", "workspace_missing"
}

func gitOutput(dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git %s timeout", strings.Join(args, " "))
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func actionableIssueFromRule(rule policy.Rule) ActionableIssue {
	return ActionableIssue{
		RuleID:          rule.RuleID,
		Owner:           rule.Owner,
		Category:        rule.Category,
		Severity:        normalizeSeverity(rule.Severity),
		CanonicalSource: rule.CanonicalSource,
		Blocking:        rule.Blocking || normalizeSeverity(rule.Severity) == "error",
		Autofix:         rule.Autofix,
		SafeCommand:     rule.SafeCommand,
	}
}

func yamlFieldValues(path string, field string, allowWildcard bool) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	return fieldValues(document, strings.Split(field, "."), allowWildcard), nil
}

func frontmatterFieldValues(path string, field string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	frontmatter, ok := extractFrontmatter(string(content))
	if !ok {
		return nil, fmt.Errorf("frontmatter missing")
	}
	var document any
	if err := yaml.Unmarshal([]byte(frontmatter), &document); err != nil {
		return nil, err
	}
	return fieldValues(document, strings.Split(field, "."), false), nil
}

func extractFrontmatter(content string) (string, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") {
		return "", false
	}
	rest := strings.TrimPrefix(content, "---\n")
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return "", false
	}
	return rest[:index], true
}

func fieldValues(value any, parts []string, allowWildcard bool) []string {
	if len(parts) == 0 || (len(parts) == 1 && strings.TrimSpace(parts[0]) == "") {
		return []string{scalarString(value)}
	}
	head := parts[0]
	tail := parts[1:]
	if head == "*" && allowWildcard {
		result := []string{}
		for _, item := range asSlice(value) {
			result = append(result, fieldValues(item, tail, allowWildcard)...)
		}
		return result
	}
	item, ok := asMap(value)[head]
	if !ok {
		return nil
	}
	return fieldValues(item, tail, allowWildcard)
}

func asMap(value any) map[string]any {
	result := map[string]any{}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		for key, item := range typed {
			result[fmt.Sprint(key)] = item
		}
	}
	return result
}

func asSlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(typed)
	}
}

func allEqual(values []string, expected string) bool {
	for _, value := range values {
		if value != expected {
			return false
		}
	}
	return true
}

func issueFromPolicy(item policy.Issue) ActionableIssue {
	severity := normalizeSeverity(item.Level)
	issue := ActionableIssue{
		RuleID:      "policy.registry",
		Owner:       "harness",
		Category:    "policy_registry",
		Severity:    severity,
		Code:        item.Code,
		Message:     item.Message,
		Blocking:    severity == "error",
		Autofix:     false,
		SafeCommand: "",
	}
	issue.Bucket = chooseBucket(issue, false)
	return issue
}

func issueFromState(item state.Issue) ActionableIssue {
	severity := normalizeSeverity(item.Level)
	issue := ActionableIssue{
		RuleID:          fallback(item.RuleID, "state.inventory"),
		Owner:           "harness",
		Category:        "state",
		Severity:        severity,
		CanonicalSource: item.File,
		Code:            item.Code,
		Message:         item.Message,
		Blocking:        severity == "error",
		Autofix:         false,
		SafeCommand:     "",
	}
	issue.Bucket = chooseBucket(issue, false)
	return issue
}

func issueFromDualReadRule(rule policy.Rule, item state.Issue) ActionableIssue {
	severity := normalizeSeverity(item.Level)
	if normalizeSeverity(rule.Severity) == "error" {
		severity = "error"
	}
	issue := ActionableIssue{
		RuleID:          fallback(rule.RuleID, fallback(item.RuleID, "state.dual_read")),
		Owner:           fallback(rule.Owner, "harness"),
		Category:        fallback(rule.Category, "state"),
		Severity:        severity,
		CanonicalSource: fallback(item.File, rule.CanonicalSource),
		Code:            item.Code,
		Message:         item.Message,
		Blocking:        rule.Blocking || severity == "error",
		Autofix:         rule.Autofix,
		SafeCommand:     rule.SafeCommand,
	}
	issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
	return issue
}

func isDualReadStateIssue(item state.Issue) bool {
	return strings.HasPrefix(item.RuleID, "state.dual_read.") || strings.HasPrefix(item.Code, "state-dual-read-")
}

func bucketize(issues []ActionableIssue) checkBuckets {
	buckets := checkBuckets{
		BlockingNow:          []ActionableIssue{},
		SafeAutofixes:        []ActionableIssue{},
		HumanDecisionsNeeded: []ActionableIssue{},
		ArchiveOnly:          []ActionableIssue{},
	}
	for _, issue := range issues {
		switch issue.Bucket {
		case "blocking_now":
			buckets.BlockingNow = append(buckets.BlockingNow, issue)
		case "safe_autofixes":
			buckets.SafeAutofixes = append(buckets.SafeAutofixes, issue)
		case "archive_or_history_only":
			buckets.ArchiveOnly = append(buckets.ArchiveOnly, issue)
		default:
			buckets.HumanDecisionsNeeded = append(buckets.HumanDecisionsNeeded, issue)
		}
	}
	return buckets
}

func chooseBucket(issue ActionableIssue, archiveOnly bool) string {
	if archiveOnly {
		return "archive_or_history_only"
	}
	if issue.Blocking {
		return "blocking_now"
	}
	if issue.Autofix && strings.TrimSpace(issue.SafeCommand) != "" {
		return "safe_autofixes"
	}
	return "human_decisions_needed"
}

func summarizeStatus(buckets checkBuckets) string {
	if len(buckets.BlockingNow) > 0 {
		return "blocked"
	}
	if len(buckets.SafeAutofixes) > 0 || len(buckets.HumanDecisionsNeeded) > 0 {
		return "degraded"
	}
	return "ok"
}

func normalizeSeverity(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "error" {
		return "error"
	}
	return "warning"
}

func statPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func statDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--profile":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --profile")
			}
			opts.Profile = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func usageText() string {
	return `Usage:
  anton check run [--profile PROFILE] [--json]
  anton check repair-plan [--profile PROFILE] [--json]
`
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func writeResponse(command string, data any, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{OK: exitCode == 0, Command: command, Data: data}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	switch typed := data.(type) {
	case runData:
		_, _ = fmt.Fprintf(stdout, "Anton %s\nStatus: %s\nIssues: %d\n", command, typed.Summary.Status, typed.Summary.TotalIssues)
	case repairPlanData:
		_, _ = fmt.Fprintf(stdout, "Anton %s\nStatus: %s\nActions: %d\n", command, typed.Summary.Status, len(typed.Actions))
	}
	return exitCode
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := response{
		OK:      false,
		Command: command,
		Error: &errorPayload{
			Code:    code,
			Message: message,
		},
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
