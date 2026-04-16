package versioncmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Andrew0613/Anton/internal/buildinfo"
)

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *responseData `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type responseData struct {
	Version string `json:"version"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return writeError("usage", fmt.Sprintf("unexpected argument: %s", arg), asJSON, stdout, stderr, 2)
		}
	}

	payload := response{
		OK:      true,
		Command: "version",
		Data: &responseData{
			Version: buildinfo.Version,
		},
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "anton %s\n", buildinfo.Version)
	return 0
}

func writeError(code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	if asJSON {
		payload := response{
			OK:      false,
			Command: "version",
			Error: &errorPayload{
				Code:    code,
				Message: message,
			},
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}

	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
