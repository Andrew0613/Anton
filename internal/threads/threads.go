package threads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

type options struct {
	JSON    bool
	Limit   int
	Project string
}

type adapterInfo struct {
	Adapter          string `json:"adapter"`
	BinaryPath       string `json:"binary_path"`
	Discovery        string `json:"discovery"`
	WorkingDirectory string `json:"working_directory"`
	ConfigPath       string `json:"config_path"`
	ConfigSource     string `json:"config_source"`
	ThreadsStrategy  string `json:"threads_default_project_strategy"`
	Project          string `json:"project,omitempty"`
	ProjectSource    string `json:"project_source,omitempty"`
	ScopeWarning     string `json:"scope_warning,omitempty"`
}

type responseData struct {
	Adapter adapterInfo `json:"adapter"`
	Raw     any         `json:"raw"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *responseData `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, environ)
	case "recent":
		return runRecent(args[1:], stdout, stderr, environ)
	case "insights":
		return runInsights(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown threads command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args, false, 0)
	if err != nil {
		return writeError("threads doctor", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("threads doctor", "threads-doctor-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("threads doctor", "threads-doctor-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	binaryPath, discovery, err := locateCodexThreads(environ)
	if err != nil {
		return writeError("threads doctor", "threads-doctor-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	raw, err := executeJSON(binaryPath, environ, wd, "doctor", "--json")
	if err != nil {
		return writeError("threads doctor", "threads-doctor-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := responseData{
		Adapter: adapterInfo{
			Adapter:          resolved.Definition.Name(),
			BinaryPath:       binaryPath,
			Discovery:        discovery,
			WorkingDirectory: wd,
			ConfigPath:       resolved.Config.Path,
			ConfigSource:     resolved.Config.Source(),
			ThreadsStrategy:  resolved.Config.Threads.DefaultProjectStrategy,
		},
		Raw: raw,
	}
	return writeResponse("threads doctor", data, opts.JSON, stdout, 0)
}

func runRecent(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args, true, 20)
	if err != nil {
		return writeError("threads recent", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("threads recent", "threads-recent-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("threads recent", "threads-recent-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	binaryPath, discovery, err := locateCodexThreads(environ)
	if err != nil {
		return writeError("threads recent", "threads-recent-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	projectSpec := resolved.Definition.ResolveThreadsProject(resolved.Context, environ, opts.Project)
	project := projectSpec.Name
	source := projectSpec.Source
	commandArgs := []string{
		"threads", "recent",
		"--json",
		"--limit", strconv.Itoa(opts.Limit),
		"--cwd", wd,
	}
	if project != "" {
		commandArgs = append(commandArgs, "--project", project)
	}

	raw, err := executeJSON(binaryPath, environ, wd, commandArgs...)
	if err != nil {
		return writeError("threads recent", "threads-recent-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := responseData{
		Adapter: adapterInfo{
			Adapter:          resolved.Definition.Name(),
			BinaryPath:       binaryPath,
			Discovery:        discovery,
			WorkingDirectory: wd,
			ConfigPath:       resolved.Config.Path,
			ConfigSource:     resolved.Config.Source(),
			ThreadsStrategy:  resolved.Config.Threads.DefaultProjectStrategy,
			Project:          project,
			ProjectSource:    source,
			ScopeWarning:     scopeWarning(project),
		},
		Raw: raw,
	}
	return writeResponse("threads recent", data, opts.JSON, stdout, 0)
}

func runInsights(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args, true, 50)
	if err != nil {
		return writeError("threads insights", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("threads insights", "threads-insights-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("threads insights", "threads-insights-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	binaryPath, discovery, err := locateCodexThreads(environ)
	if err != nil {
		return writeError("threads insights", "threads-insights-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	projectSpec := resolved.Definition.ResolveThreadsProject(resolved.Context, environ, opts.Project)
	project := projectSpec.Name
	source := projectSpec.Source
	commandArgs := []string{
		"insights",
		"--json",
		"--limit", strconv.Itoa(opts.Limit),
	}
	if project != "" {
		commandArgs = append(commandArgs, "--project", project)
	}

	raw, err := executeJSON(binaryPath, environ, wd, commandArgs...)
	if err != nil {
		return writeError("threads insights", "threads-insights-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := responseData{
		Adapter: adapterInfo{
			Adapter:          resolved.Definition.Name(),
			BinaryPath:       binaryPath,
			Discovery:        discovery,
			WorkingDirectory: wd,
			ConfigPath:       resolved.Config.Path,
			ConfigSource:     resolved.Config.Source(),
			ThreadsStrategy:  resolved.Config.Threads.DefaultProjectStrategy,
			Project:          project,
			ProjectSource:    source,
			ScopeWarning:     scopeWarning(project),
		},
		Raw: raw,
	}
	return writeResponse("threads insights", data, opts.JSON, stdout, 0)
}

func parseOptions(args []string, allowLimit bool, defaultLimit int) (options, error) {
	opts := options{Limit: defaultLimit}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--project":
			index++
			if index >= len(args) {
				return options{}, fmt.Errorf("missing value for --project")
			}
			opts.Project = args[index]
		case "--limit":
			if !allowLimit {
				return options{}, fmt.Errorf("--limit is not supported for this command")
			}
			index++
			if index >= len(args) {
				return options{}, fmt.Errorf("missing value for --limit")
			}
			value, err := strconv.Atoi(args[index])
			if err != nil || value <= 0 {
				return options{}, fmt.Errorf("invalid --limit value %q", args[index])
			}
			opts.Limit = value
		default:
			return options{}, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func locateCodexThreads(environ []string) (string, string, error) {
	env := envMap(environ)
	if path := findOnPath("codex-threads", env["PATH"]); path != "" {
		return path, "path", nil
	}

	if home := env["HOME"]; home != "" {
		fallback := filepath.Join(home, ".local", "bin", "codex-threads")
		if isExecutableFile(fallback) {
			return fallback, "home-local-bin", nil
		}
	}

	return "", "", fmt.Errorf("codex-threads is not available on PATH and no ~/.local/bin fallback was found")
}

func findOnPath(binary string, pathValue string) string {
	for _, entry := range strings.Split(pathValue, string(os.PathListSeparator)) {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		candidate := filepath.Join(entry, binary)
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func executeJSON(binaryPath string, environ []string, wd string, args ...string) (any, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = wd
	cmd.Env = environ

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}

	var payload any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return nil, fmt.Errorf("decode codex-threads json: %w", err)
	}
	return payload, nil
}

func scopeWarning(project string) string {
	if project == "" {
		return "no project could be inferred, so the codex-threads query ran without a project scope"
	}
	return ""
}

func envMap(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton threads doctor [--json]
  anton threads recent [--json] [--limit N] [--project NAME]
  anton threads insights [--json] [--limit N] [--project NAME]
`
}

func writeResponse(command string, data responseData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{
		OK:      true,
		Command: command,
		Data:    &data,
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}

	_, _ = fmt.Fprintf(stdout, "Anton %s\n", command)
	_, _ = fmt.Fprintf(stdout, "Adapter: %s\n", data.Adapter.Adapter)
	_, _ = fmt.Fprintf(stdout, "Binary: %s\n", data.Adapter.BinaryPath)
	_, _ = fmt.Fprintf(stdout, "Discovery: %s\n", data.Adapter.Discovery)
	_, _ = fmt.Fprintf(stdout, "Config source: %s\n", data.Adapter.ConfigSource)
	_, _ = fmt.Fprintf(stdout, "Working dir: %s\n", data.Adapter.WorkingDirectory)
	if data.Adapter.Project != "" {
		_, _ = fmt.Fprintf(stdout, "Project: %s (%s)\n", data.Adapter.Project, data.Adapter.ProjectSource)
	}
	if data.Adapter.ScopeWarning != "" {
		_, _ = fmt.Fprintf(stdout, "Warning: %s\n", data.Adapter.ScopeWarning)
	}

	encoded, _ := json.MarshalIndent(data.Raw, "", "  ")
	_, _ = fmt.Fprintf(stdout, "\nRaw Payload\n%s\n", string(encoded))
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
