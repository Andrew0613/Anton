package contextcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/doctor"
)

type options struct {
	JSON    bool
	Explain bool
}

type payload struct {
	Contract contract.ContractV1 `json:"contract"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *payload      `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("context", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	contractData, err := doctor.CollectContract(environ)
	if err != nil {
		return writeError("context", "context-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	output := response{
		OK:      contractData.Summary.BlockedCount == 0,
		Command: "context",
		Data:    &payload{Contract: contractData},
	}

	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(output)
	} else {
		renderHuman(stdout, output, opts.Explain)
	}

	if output.OK && contractData.Summary.DegradedCount == 0 {
		return 0
	}
	return 1
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.JSON = true
		case "--explain":
			opts.Explain = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func renderHuman(stdout io.Writer, output response, explain bool) {
	if output.Data == nil {
		return
	}
	data := output.Data.Contract
	_, _ = fmt.Fprintf(stdout, "Anton context\n")
	_, _ = fmt.Fprintf(stdout, "Status: %s\n\n", data.Summary.Status)
	_, _ = fmt.Fprintf(stdout, "Repo: %s\n", fallback(data.Context.RepositoryRoot, data.Context.WorkingDirectory))
	_, _ = fmt.Fprintf(stdout, "Workspace: %s\n", data.Context.WorkspaceKind)
	if data.Context.GitBranch != "" {
		_, _ = fmt.Fprintf(stdout, "Branch: %s\n", data.Context.GitBranch)
	}
	_, _ = fmt.Fprintf(stdout, "Config: %s (%s)\n", data.Config.Source, data.Config.Path)
	_, _ = fmt.Fprintf(stdout, "Entrypoint: %s\n", data.Config.EntrypointPath)
	_, _ = fmt.Fprintf(stdout, "Tasks root: %s\n", data.Config.TasksRoot)
	if data.TaskIdentity.Conflict {
		_, _ = fmt.Fprintf(stdout, "Task identity: conflict (%s)\n", strings.Join(data.TaskIdentity.ConflictValues, ", "))
	} else if data.TaskIdentity.Resolved != "" {
		_, _ = fmt.Fprintf(stdout, "Task identity: %s\n", data.TaskIdentity.Resolved)
	} else {
		_, _ = fmt.Fprintf(stdout, "Task identity: unresolved\n")
	}
	if explain {
		_, _ = fmt.Fprintf(stdout, "\nPrompt Contract\n%s\n", data.PromptContract)
	}
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallbackValue
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
