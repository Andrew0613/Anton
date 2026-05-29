package task

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/state"
)

type options struct {
	JSON     bool
	TaskID   string
	State    string
	DualRead bool
}

type commandData struct {
	Adapter          string            `json:"adapter"`
	WorkingDirectory string            `json:"working_directory"`
	RepositoryRoot   string            `json:"repository_root,omitempty"`
	StateRoot        string            `json:"state_root"`
	TasksDir         string            `json:"tasks_dir"`
	SourceRevision   string            `json:"source_revision,omitempty"`
	Task             *state.TaskRecord `json:"task,omitempty"`
	Inventory        *inventorySummary `json:"inventory,omitempty"`
	Issues           []state.Issue     `json:"issues,omitempty"`
}

type inventorySummary struct {
	State       string             `json:"state"`
	Total       int                `json:"total"`
	ActiveCount int                `json:"active_count"`
	Tasks       []state.TaskRecord `json:"tasks"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *commandData  `json:"data,omitempty"`
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
	case "resolve":
		return runResolve(args[1:], stdout, stderr, environ)
	case "list":
		return runList(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown task command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runResolve(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseResolveOptions(args)
	if err != nil {
		return writeError("task resolve", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	wd, err := os.Getwd()
	if err != nil {
		return writeError("task resolve", "task-resolve-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("task resolve", "task-resolve-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	record, issues, err := state.ResolveTask(resolved, opts.TaskID, opts.DualRead)
	if err != nil {
		return writeError("task resolve", "task-resolve-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	inventory, err := state.LoadInventory(resolved, opts.DualRead)
	if err != nil {
		return writeError("task resolve", "task-resolve-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	issues = append(issues, inventory.Issues...)
	ok := len(errorIssues(issues)) == 0 && strings.TrimSpace(record.TaskID) != ""
	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: resolved.Context.WorkingDirectory,
		RepositoryRoot:   resolved.Context.RepositoryRoot,
		StateRoot:        inventory.StateRoot,
		TasksDir:         inventory.TasksDir,
		SourceRevision:   inventory.SourceRevision,
		Issues:           dedupeIssues(issues),
	}
	if strings.TrimSpace(record.TaskID) != "" {
		recordCopy := record
		data.Task = &recordCopy
	}
	exitCode := 0
	if !ok {
		exitCode = 1
	}
	return writeResponse("task resolve", data, opts.JSON, stdout, exitCode)
}

func runList(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseListOptions(args)
	if err != nil {
		return writeError("task list", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	wd, err := os.Getwd()
	if err != nil {
		return writeError("task list", "task-list-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("task list", "task-list-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	inventory, err := state.LoadInventory(resolved, opts.DualRead)
	if err != nil {
		return writeError("task list", "task-list-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	selected := inventory.Tasks
	if opts.State == "active" {
		selected = inventory.Active
	}
	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: resolved.Context.WorkingDirectory,
		RepositoryRoot:   resolved.Context.RepositoryRoot,
		StateRoot:        inventory.StateRoot,
		TasksDir:         inventory.TasksDir,
		SourceRevision:   inventory.SourceRevision,
		Inventory: &inventorySummary{
			State:       opts.State,
			Total:       len(selected),
			ActiveCount: len(inventory.Active),
			Tasks:       selected,
		},
		Issues: inventory.Issues,
	}
	exitCode := 0
	if len(errorIssues(inventory.Issues)) > 0 {
		exitCode = 1
	}
	return writeResponse("task list", data, opts.JSON, stdout, exitCode)
}

func parseResolveOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--task":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --task")
			}
			opts.TaskID = args[index]
		case "--dual-read":
			opts.DualRead = true
		default:
			if strings.HasPrefix(args[index], "-") {
				return opts, fmt.Errorf("unexpected argument: %s", args[index])
			}
			if opts.TaskID != "" {
				return opts, fmt.Errorf("task id provided more than once")
			}
			opts.TaskID = args[index]
		}
	}
	return opts, nil
}

func parseListOptions(args []string) (options, error) {
	opts := options{State: "active"}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--state":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --state")
			}
			opts.State = strings.TrimSpace(args[index])
		case "--dual-read":
			opts.DualRead = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.State != "active" && opts.State != "all" {
		return opts, fmt.Errorf("--state must be one of: active, all")
	}
	return opts, nil
}

func usageText() string {
	return `Usage:
  anton task resolve [TASK_ID|--task TASK_ID] [--dual-read] [--json]
  anton task list [--state active|all] [--dual-read] [--json]
`
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func writeResponse(command string, data commandData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{OK: exitCode == 0, Command: command, Data: &data}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	if data.Task != nil {
		_, _ = fmt.Fprintf(stdout, "Anton %s\nTask: %s\nLifecycle: %s\n", command, data.Task.TaskID, data.Task.Lifecycle)
	} else if data.Inventory != nil {
		_, _ = fmt.Fprintf(stdout, "Anton %s\nState: %s\nTasks: %d\n", command, data.Inventory.State, data.Inventory.Total)
	}
	if len(data.Issues) > 0 {
		_, _ = fmt.Fprintf(stdout, "Issues: %d\n", len(data.Issues))
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

func errorIssues(issues []state.Issue) []state.Issue {
	result := make([]state.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Level == "error" {
			result = append(result, issue)
		}
	}
	return result
}

func dedupeIssues(issues []state.Issue) []state.Issue {
	seen := map[string]bool{}
	result := make([]state.Issue, 0, len(issues))
	for _, issue := range issues {
		key := issue.Level + "|" + issue.Code + "|" + issue.File + "|" + issue.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, issue)
	}
	return result
}
