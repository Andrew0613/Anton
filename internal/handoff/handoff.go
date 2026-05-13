package handoff

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/doctor"
)

type options struct {
	JSON      bool
	Source    string
	SessionID string
}

type persistOptions struct {
	JSON         bool
	WorktreeRoot string
	RunDir       string
	DryRun       bool
}

type pack struct {
	Objective              string              `json:"objective"`
	Scope                  []string            `json:"scope"`
	TaskID                 string              `json:"task_id"`
	Lifecycle              string              `json:"lifecycle"`
	FinishState            string              `json:"finish_state"`
	ExpectedDeliverables   int                 `json:"expected_deliverable_count"`
	Blockers               int                 `json:"blocker_count"`
	NextStep               string              `json:"next_step"`
	ValidationReceiptCount int                 `json:"validation_receipt_count"`
	AttemptReceiptCount    int                 `json:"attempt_receipt_count"`
	StatusPath             string              `json:"status_path"`
	Contract               contract.ContractV1 `json:"contract"`
	TaskStatus             taskStatusSummary   `json:"task_status"`
	Git                    gitSummary          `json:"git"`
	ValidationReceipts     []handoffEvidence   `json:"validation_receipts,omitempty"`
	BlockerDetails         []string            `json:"blockers,omitempty"`
	UserDecisions          []string            `json:"user_decisions,omitempty"`
	NextCommands           []string            `json:"next_commands,omitempty"`
	Source                 sourceSummary       `json:"source"`
	SourceSnippets         []sourceSnippet     `json:"source_snippets,omitempty"`
	Warnings               []string            `json:"warnings,omitempty"`
	GeneratedAt            string              `json:"generated_at"`
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
	case "build":
		return runBuild(args[1:], stdout, stderr, environ)
	case "persist-results":
		return runPersistResults(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown handoff command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runBuild(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("handoff build", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	contractData, err := doctor.CollectContract(environ)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, time.Now().UTC())
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	statusPath := bundle.StatusPath()
	snapshot, err := resolved.Definition.ReadStatus(statusPath)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	taskPlanPath := filepath.Join(bundle.Root, "task_plan.md")
	planContent, err := os.ReadFile(taskPlanPath)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", fmt.Sprintf("read %s: %v", taskPlanPath, err), opts.JSON, stdout, stderr, 1)
	}

	statusDetails, err := readTaskStatusSummary(statusPath, snapshot)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	git := collectGitSummary(resolved.Context.RepositoryRoot, resolved.Context.GitBranch)
	source, snippets, sourceWarnings, err := collectSourceSnippets(opts.Source, opts.SessionID, environ)
	if err != nil {
		return writeError("handoff build", "handoff-build-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	warnings := handoffWarnings(snapshot, contractData)
	warnings = append(warnings, git.Warnings...)
	warnings = append(warnings, sourceWarnings...)

	output := pack{
		Objective:              extractObjective(string(planContent)),
		Scope:                  resolved.Context.ScopePaths,
		TaskID:                 snapshot.TaskID,
		Lifecycle:              snapshot.Lifecycle,
		FinishState:            snapshot.FinishState,
		ExpectedDeliverables:   snapshot.ExpectedDeliverableCount,
		Blockers:               snapshot.BlockerCount,
		NextStep:               snapshot.NextStep,
		ValidationReceiptCount: snapshot.ValidationCount,
		AttemptReceiptCount:    snapshot.AttemptCount,
		StatusPath:             statusPath,
		Contract:               contractData,
		TaskStatus:             statusDetails,
		Git:                    git,
		ValidationReceipts:     statusDetails.ValidationReceipts,
		BlockerDetails:         statusDetails.Blockers,
		UserDecisions:          extractUserDecisions(statusDetails, snippets, string(planContent)),
		NextCommands:           nextCommands(snapshot),
		Source:                 source,
		SourceSnippets:         snippets,
		Warnings:               warnings,
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
	}

	resp := response{
		OK:      true,
		Command: "handoff build",
		Data:    output,
	}

	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(resp)
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "Anton handoff build\n")
	_, _ = fmt.Fprintf(stdout, "Task ID: %s\n", output.TaskID)
	_, _ = fmt.Fprintf(stdout, "Objective: %s\n", output.Objective)
	_, _ = fmt.Fprintf(stdout, "Lifecycle: %s (%s)\n", output.Lifecycle, output.FinishState)
	_, _ = fmt.Fprintf(stdout, "Scope: %s\n", strings.Join(output.Scope, ", "))
	_, _ = fmt.Fprintf(stdout, "Next step: %s\n", output.NextStep)
	if len(output.NextCommands) > 0 {
		_, _ = fmt.Fprintf(stdout, "Next commands: %s\n", strings.Join(output.NextCommands, " && "))
	}
	_, _ = fmt.Fprintf(stdout, "Receipts: attempts=%d validations=%d\n", output.AttemptReceiptCount, output.ValidationReceiptCount)
	return 0
}

func runPersistResults(args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parsePersistOptions(args)
	if err != nil {
		return writeError("handoff persist-results", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	plan, err := buildPersistPlan(opts.WorktreeRoot, opts.RunDir, opts.DryRun)
	if err != nil {
		return writeError("handoff persist-results", "persist-results-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	resp := response{
		OK:      true,
		Command: "handoff persist-results",
		Data:    plan,
	}
	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(resp)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "Anton handoff persist-results\n")
	_, _ = fmt.Fprintf(stdout, "Dry run: %t\n", plan.DryRun)
	_, _ = fmt.Fprintf(stdout, "Files planned: %d\n", plan.FileCount)
	_, _ = fmt.Fprintf(stdout, "Bytes planned: %d\n", plan.ByteCount)
	return 0
}

func handoffWarnings(snapshot adapter.StatusSnapshot, contractData contract.ContractV1) []string {
	warnings := []string{}
	if contractData.TaskIdentity.Resolved != "" && snapshot.TaskID != "" && contractData.TaskIdentity.Resolved != snapshot.TaskID {
		warnings = append(warnings, fmt.Sprintf("contract task identity %q differs from status task id %q", contractData.TaskIdentity.Resolved, snapshot.TaskID))
	}
	return warnings
}

func parseOptions(args []string) (options, error) {
	opts := options{Source: "manual"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--json":
			opts.JSON = true
		case "--source":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--source requires a value")
			}
			opts.Source = strings.TrimSpace(args[index])
		case "--session-id":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--session-id requires a value")
			}
			opts.SessionID = strings.TrimSpace(args[index])
		default:
			if strings.HasPrefix(arg, "--source=") {
				opts.Source = strings.TrimSpace(strings.TrimPrefix(arg, "--source="))
				continue
			}
			if strings.HasPrefix(arg, "--session-id=") {
				opts.SessionID = strings.TrimSpace(strings.TrimPrefix(arg, "--session-id="))
				continue
			}
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	switch opts.Source {
	case "", "manual":
		opts.Source = "manual"
	case "codex", "claude":
		if opts.SessionID == "" {
			return opts, fmt.Errorf("--session-id is required when --source is %s", opts.Source)
		}
	default:
		return opts, fmt.Errorf("--source must be one of manual, codex, or claude")
	}
	return opts, nil
}

func parsePersistOptions(args []string) (persistOptions, error) {
	opts := persistOptions{DryRun: true}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--json":
			opts.JSON = true
		case "--dry-run":
			opts.DryRun = true
		case "--worktree-root":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--worktree-root requires a value")
			}
			opts.WorktreeRoot = strings.TrimSpace(args[index])
		case "--run-dir":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--run-dir requires a value")
			}
			opts.RunDir = strings.TrimSpace(args[index])
		default:
			if strings.HasPrefix(arg, "--worktree-root=") {
				opts.WorktreeRoot = strings.TrimSpace(strings.TrimPrefix(arg, "--worktree-root="))
				continue
			}
			if strings.HasPrefix(arg, "--run-dir=") {
				opts.RunDir = strings.TrimSpace(strings.TrimPrefix(arg, "--run-dir="))
				continue
			}
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if opts.WorktreeRoot == "" {
		return opts, fmt.Errorf("--worktree-root is required")
	}
	if opts.RunDir == "" {
		return opts, fmt.Errorf("--run-dir is required")
	}
	return opts, nil
}

func extractObjective(content string) string {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "## Goal" {
			continue
		}
		for cursor := index + 1; cursor < len(lines); cursor++ {
			value := strings.TrimSpace(lines[cursor])
			if value == "" {
				continue
			}
			if strings.HasPrefix(value, "#") {
				break
			}
			return value
		}
	}
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value != "" && !strings.HasPrefix(value, "#") {
			return value
		}
	}
	return "No explicit objective recorded in task_plan.md"
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton handoff build [--source manual|codex|claude] [--session-id ID] [--json]
  anton handoff persist-results --worktree-root PATH --run-dir PATH [--dry-run] [--json]
`
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
