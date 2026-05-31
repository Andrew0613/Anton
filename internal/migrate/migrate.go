package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/workspace"
)

type options struct {
	JSON   bool
	Target string
}

type schema struct {
	Version int    `json:"version"`
	Locked  bool   `json:"locked"`
	Reason  string `json:"reason,omitempty"`
}

type commandData struct {
	Adapter          string                  `json:"adapter"`
	WorkingDirectory string                  `json:"working_directory"`
	RepositoryRoot   string                  `json:"repository_root,omitempty"`
	ConfigPath       string                  `json:"config_path"`
	ConfigSource     string                  `json:"config_source"`
	TargetSchema     *schema                 `json:"target_schema,omitempty"`
	Target           *workspace.TargetStatus `json:"target,omitempty"`
	Refs             *workspace.RefsReport   `json:"refs,omitempty"`
	Status           string                  `json:"status"`
	BlockedReason    string                  `json:"blocked_reason,omitempty"`
	Recommendation   string                  `json:"recommendation,omitempty"`
	ProposedChanges  []string                `json:"proposed_changes"`
	ReadOnly         bool                    `json:"read_only"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *commandData  `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}
	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdout, stderr, environ)
	case "readiness":
		return runReadiness(args[1:], stdout, stderr, environ)
	case "apply":
		opts, err := parseOptions(args[1:])
		if err != nil {
			return writeError("migrate apply", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
		}
		return writeError("migrate apply", "not-approved", "migrate apply is not approved until snapshot and rollback behavior is specified", opts.JSON, stdout, stderr, 2)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown migrate command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runReadiness(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("migrate readiness", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	report, err := workspace.BuildRefsReport(environ, opts.Target)
	if err != nil {
		return writeError("migrate readiness", "migrate-readiness-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	status := report.Summary.Status
	blockedReason := ""
	if report.Summary.Blockers > 0 {
		blockedReason = "target is not ready for migration; fix blockers before planning file moves"
	}
	target := report.Target
	data := commandData{
		Adapter:          report.Adapter,
		WorkingDirectory: report.WorkingDirectory,
		RepositoryRoot:   report.RepositoryRoot,
		ConfigPath:       report.ConfigPath,
		ConfigSource:     report.ConfigSource,
		Target:           &target,
		Refs:             &report,
		Status:           status,
		BlockedReason:    blockedReason,
		Recommendation:   report.Summary.Recommendation,
		ProposedChanges:  []string{},
		ReadOnly:         true,
	}
	exitCode := 0
	if report.Summary.Blockers > 0 {
		exitCode = 1
	}
	return writeResponse("migrate readiness", data, opts.JSON, stdout, exitCode)
}

func runPlan(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("migrate plan", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	wd, err := os.Getwd()
	if err != nil {
		return writeError("migrate plan", "migrate-plan-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("migrate plan", "migrate-plan-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	targetSchema := schema{
		Version: resolved.Config.MigrateTargetSchemaVersion(),
		Locked:  resolved.Config.MigrateTargetSchemaLocked(),
		Reason:  resolved.Config.MigrateTargetSchemaReason(),
	}
	if !targetSchema.Locked {
		data := commandData{
			Adapter:          resolved.Definition.Name(),
			WorkingDirectory: wd,
			ConfigPath:       resolved.Config.Path,
			ConfigSource:     resolved.Config.Source(),
			TargetSchema:     &targetSchema,
			Status:           "blocked",
			BlockedReason:    targetSchema.Reason + "; migrate plan must not invent target fields",
			ProposedChanges:  []string{},
			ReadOnly:         true,
		}
		return writeResponse("migrate plan", data, opts.JSON, stdout, 1)
	}

	target := strings.TrimSpace(opts.Target)
	if target == "" {
		target = resolved.Config.MigrateDefaultTarget()
	}
	report, err := workspace.BuildRefsReport(environ, target)
	if err != nil {
		return writeError("migrate plan", "migrate-plan-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	blockedReason := ""
	if report.Summary.Blockers > 0 {
		blockedReason = "target is not ready for migration; fix blockers before planning file moves"
	}
	targetStatus := report.Target
	data := commandData{
		Adapter:          report.Adapter,
		WorkingDirectory: report.WorkingDirectory,
		RepositoryRoot:   report.RepositoryRoot,
		ConfigPath:       report.ConfigPath,
		ConfigSource:     report.ConfigSource,
		TargetSchema:     &targetSchema,
		Target:           &targetStatus,
		Refs:             &report,
		Status:           report.Summary.Status,
		BlockedReason:    blockedReason,
		Recommendation:   report.Summary.Recommendation,
		ProposedChanges:  proposedPlanSteps(report),
		ReadOnly:         true,
	}
	exitCode := 0
	if report.Summary.Blockers > 0 {
		exitCode = 1
	}
	return writeResponse("migrate plan", data, opts.JSON, stdout, exitCode)
}

func proposedPlanSteps(report workspace.RefsReport) []string {
	steps := []string{
		fmt.Sprintf("metadata-only readiness scan for %s", report.Target.Relative),
		"no file moves are approved by anton migrate plan",
	}
	if report.Summary.ReferenceHits > 0 {
		steps = append(steps, fmt.Sprintf("review %d textual reference(s) before any target move", report.Summary.ReferenceHits))
	}
	if report.TaskBundle.TargetOverlaps {
		steps = append(steps, "preserve or retarget task bundle state before moving the target")
	}
	for _, worktree := range report.Worktrees {
		if worktree.OverlapsTarget && !worktree.Current {
			steps = append(steps, "coordinate overlapping non-current worktrees before moving the target")
			break
		}
	}
	return steps
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--target":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --target")
			}
			opts.Target = args[index]
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton migrate plan [--target PATH] [--json]
  anton migrate readiness --target PATH [--json]
`
}

func writeResponse(command string, data commandData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{OK: exitCode == 0, Command: command, Data: &data}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stdout, "Anton %s\nStatus: %s\n", command, data.Status)
	if strings.TrimSpace(data.BlockedReason) != "" {
		_, _ = fmt.Fprintf(stdout, "Blocked: %s\n", data.BlockedReason)
	}
	if data.Target != nil {
		_, _ = fmt.Fprintf(stdout, "Target: %s\nRecommendation: %s\n", data.Target.Relative, data.Recommendation)
	}
	return exitCode
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := response{OK: false, Command: command, Error: &errorPayload{Code: code, Message: message}}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
