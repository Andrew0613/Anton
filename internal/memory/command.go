package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type options struct {
	JSON bool
}

type updateOptions struct {
	options
	Key        string
	Value      string
	Source     string
	Freshness  string
	Confidence string
	Author     string
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type statusResponse struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *StatusData   `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type updateResponse struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *UpdateData   `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "status":
		return runStatus(args[1:], stdout, stderr, environ)
	case "update":
		return runUpdate(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown memory command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runStatus(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseStatusOptions(args)
	if err != nil {
		return writeStatusError("memory status", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	data, err := collectStatus(environ, time.Now().UTC())
	if err != nil {
		return writeStatusError("memory status", statusErrorCode(err), err.Error(), opts.JSON, stdout, stderr, 1)
	}

	response := statusResponse{
		OK:      true,
		Command: "memory status",
		Data:    &data,
	}
	if opts.JSON {
		writeJSON(stdout, response)
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "Anton memory status\n")
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", data.Summary.Status)
	_, _ = fmt.Fprintf(stdout, "Memory path: %s\n", data.MemoryPath)
	_, _ = fmt.Fprintf(stdout, "Entries: %d stale=%d conflicts=%d warnings=%d\n", data.Summary.EntryCount, data.Summary.StaleCount, data.Summary.ConflictCount, data.Summary.WarningCount)
	for _, warning := range data.Warnings {
		_, _ = fmt.Fprintf(stdout, "WARNING %s: %s\n", warning.Code, warning.Message)
	}
	return 0
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseUpdateOptions(args)
	if err != nil {
		return writeUpdateError("memory update", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	data, err := appendEvent(environ, opts, time.Now().UTC())
	if err != nil {
		return writeUpdateError("memory update", "memory-update-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	response := updateResponse{
		OK:      true,
		Command: "memory update",
		Data:    &data,
	}
	if opts.JSON {
		writeJSON(stdout, response)
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "Anton memory update\n")
	_, _ = fmt.Fprintf(stdout, "Appended: %s\n", data.Event.Key)
	_, _ = fmt.Fprintf(stdout, "Memory path: %s\n", data.MemoryPath)
	return 0
}

func parseStatusOptions(args []string) (options, error) {
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

func parseUpdateOptions(args []string) (updateOptions, error) {
	opts := updateOptions{Confidence: "medium"}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--key":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --key")
			}
			opts.Key = strings.TrimSpace(args[index])
		case "--value":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --value")
			}
			opts.Value = strings.TrimSpace(args[index])
		case "--source":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --source")
			}
			opts.Source = strings.TrimSpace(args[index])
		case "--freshness":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --freshness")
			}
			opts.Freshness = strings.TrimSpace(args[index])
		case "--confidence":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --confidence")
			}
			opts.Confidence = strings.TrimSpace(args[index])
		case "--author":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --author")
			}
			opts.Author = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.Key == "" {
		return opts, fmt.Errorf("--key is required")
	}
	if opts.Value == "" {
		return opts, fmt.Errorf("--value is required")
	}
	if opts.Source == "" {
		return opts, fmt.Errorf("--source is required")
	}
	if opts.Confidence != "" && !validConfidences[opts.Confidence] {
		return opts, fmt.Errorf("--confidence must be one of: low, medium, high")
	}
	if opts.Freshness != "" {
		if _, err := time.Parse(time.RFC3339, opts.Freshness); err != nil {
			return opts, fmt.Errorf("--freshness must be RFC3339: %v", err)
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
  anton memory status [--json]
  anton memory update --key KEY --value VALUE --source SOURCE [--confidence low|medium|high] [--freshness RFC3339] [--author NAME] [--json]
`
}

func writeStatusError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := statusResponse{
		OK:      false,
		Command: command,
		Error: &errorPayload{
			Code:    code,
			Message: message,
		},
	}
	if asJSON {
		writeJSON(stdout, payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}

func writeUpdateError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := updateResponse{
		OK:      false,
		Command: command,
		Error: &errorPayload{
			Code:    code,
			Message: message,
		},
	}
	if asJSON {
		writeJSON(stdout, payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}

func writeJSON(stdout io.Writer, payload any) {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func statusErrorCode(err error) string {
	var corrupt CorruptError
	if errors.As(err, &corrupt) {
		return "memory-corrupt"
	}
	return "memory-status-failed"
}
