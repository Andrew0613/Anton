package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

type options struct{ JSON bool }

type schema struct {
	Version int    `json:"version"`
	Locked  bool   `json:"locked"`
	Reason  string `json:"reason,omitempty"`
}

type commandData struct {
	Adapter          string   `json:"adapter"`
	WorkingDirectory string   `json:"working_directory"`
	ConfigPath       string   `json:"config_path"`
	ConfigSource     string   `json:"config_source"`
	TargetSchema     schema   `json:"target_schema"`
	Status           string   `json:"status"`
	BlockedReason    string   `json:"blocked_reason,omitempty"`
	ProposedChanges  []string `json:"proposed_changes"`
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
	reason := "v2 config schema is not locked"
	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		ConfigPath:       resolved.Config.Path,
		ConfigSource:     resolved.Config.Source(),
		TargetSchema:     schema{Version: 2, Locked: false, Reason: reason},
		Status:           "blocked",
		BlockedReason:    reason + "; migrate plan must not invent target fields",
		ProposedChanges:  []string{},
	}
	return writeResponse("migrate plan", data, opts.JSON, stdout, 1)
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

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton migrate plan [--json]
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
