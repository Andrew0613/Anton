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
)

type options struct {
	JSON bool
}

type pack struct {
	Objective              string   `json:"objective"`
	Scope                  []string `json:"scope"`
	TaskID                 string   `json:"task_id"`
	Lifecycle              string   `json:"lifecycle"`
	FinishState            string   `json:"finish_state"`
	ExpectedDeliverables   int      `json:"expected_deliverable_count"`
	Blockers               int      `json:"blocker_count"`
	NextStep               string   `json:"next_step"`
	ValidationReceiptCount int      `json:"validation_receipt_count"`
	AttemptReceiptCount    int      `json:"attempt_receipt_count"`
	StatusPath             string   `json:"status_path"`
	GeneratedAt            string   `json:"generated_at"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *pack         `json:"data,omitempty"`
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
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
	}

	resp := response{
		OK:      true,
		Command: "handoff build",
		Data:    &output,
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
	_, _ = fmt.Fprintf(stdout, "Receipts: attempts=%d validations=%d\n", output.AttemptReceiptCount, output.ValidationReceiptCount)
	return 0
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
  anton handoff build [--json]
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
