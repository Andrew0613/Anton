package check

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/policy"
	"github.com/Andrew0613/Anton/internal/state"
)

type options struct {
	JSON bool
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
	report, err := collect(environ)
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
	report, err := collect(environ)
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

func collect(environ []string) (runData, error) {
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
	for _, rule := range registry.Rules {
		if strings.TrimSpace(rule.Check.Kind) == "" {
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

func evaluateRule(base string, rule policy.Rule) (ActionableIssue, bool) {
	issue := ActionableIssue{
		RuleID:          rule.RuleID,
		Owner:           rule.Owner,
		Category:        rule.Category,
		Severity:        normalizeSeverity(rule.Severity),
		CanonicalSource: rule.CanonicalSource,
		Blocking:        rule.Blocking || normalizeSeverity(rule.Severity) == "error",
		Autofix:         rule.Autofix,
		SafeCommand:     rule.SafeCommand,
	}
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
	default:
		issue.Code = "unsupported-rule-kind"
		issue.Message = fmt.Sprintf("unsupported policy check.kind %q", rule.Check.Kind)
		issue.Blocking = true
		issue.Autofix = false
	}
	issue.Bucket = chooseBucket(issue, rule.ArchiveOnly)
	return issue, true
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
		RuleID:      fallback(item.RuleID, "state.inventory"),
		Owner:       "harness",
		Category:    "state",
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

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.JSON = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func usageText() string {
	return `Usage:
  anton check run [--json]
  anton check repair-plan [--json]
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
