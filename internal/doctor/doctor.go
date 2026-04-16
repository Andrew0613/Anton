package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	statusOK       = "ok"
	statusDegraded = "degraded"
	statusBlocked  = "blocked"
)

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

type environment struct {
	ExecutionTarget string `json:"execution_target"`
	Host            string `json:"host"`
	OperatingSystem string `json:"operating_system"`
	Architecture    string `json:"architecture"`
	FilesystemType  string `json:"filesystem_type,omitempty"`
}

type contextContract struct {
	WorkingDirectory string   `json:"working_directory"`
	WorkspaceKind    string   `json:"workspace_kind"`
	RepositoryRoot   string   `json:"repository_root,omitempty"`
	RepositoryKind   string   `json:"repository_kind,omitempty"`
	GitBranch        string   `json:"git_branch,omitempty"`
	ScopePaths       []string `json:"scope_paths"`
}

type configContract struct {
	Path                          string   `json:"path"`
	Source                        string   `json:"source"`
	EntrypointPath                string   `json:"entrypoint_path"`
	TasksRoot                     string   `json:"tasks_root"`
	ThreadsDefaultProjectStrategy string   `json:"threads_default_project_strategy"`
	ThreadsWorkspaceRoots         []string `json:"threads_workspace_roots,omitempty"`
}

type summary struct {
	Status        string `json:"status"`
	OKCount       int    `json:"ok_count"`
	DegradedCount int    `json:"degraded_count"`
	BlockedCount  int    `json:"blocked_count"`
}

type reportData struct {
	Adapter        string          `json:"adapter"`
	Environment    environment     `json:"environment"`
	Context        contextContract `json:"context"`
	Config         configContract  `json:"config"`
	PromptContract string          `json:"prompt_contract"`
	Checks         []check         `json:"checks"`
	Summary        summary         `json:"summary"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type report struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *reportData   `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type options struct {
	JSON bool
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("doctor", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	data, err := collect(environ)
	if err != nil {
		return writeError("doctor", "doctor-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	output := report{
		OK:      data.Summary.BlockedCount == 0,
		Command: "doctor",
		Data:    &data,
	}

	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(output)
	} else {
		renderHuman(stdout, output)
	}

	if output.OK && data.Summary.DegradedCount == 0 {
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
		default:
			return options{}, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func collect(environ []string) (reportData, error) {
	envMap := envMap(environ)

	wd, err := os.Getwd()
	if err != nil {
		return reportData{}, fmt.Errorf("get working directory: %w", err)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return reportData{}, err
	}

	filesystemType := detectFilesystemType(wd)
	contextData := resolved.Context

	context := contextContract{
		WorkingDirectory: contextData.WorkingDirectory,
		WorkspaceKind:    contextData.WorkspaceKind,
		RepositoryRoot:   contextData.RepositoryRoot,
		RepositoryKind:   contextData.RepositoryKind,
		GitBranch:        contextData.GitBranch,
		ScopePaths:       contextData.ScopePaths,
	}

	checks := []check{
		checkAntonConfig(resolved.Config),
		checkWorkingDirectoryWritable(contextData.WorkingDirectory),
		checkRepositoryContext(contextData),
		checkFilesystem(filesystemType),
		checkEntrypointFile(resolved.Definition.EntrypointPath(contextData)),
		checkCodexThreads(envMap),
	}

	data := reportData{
		Adapter: resolved.Definition.Name(),
		Environment: environment{
			ExecutionTarget: contextData.ExecutionTarget,
			Host:            contextData.Host,
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
			FilesystemType:  filesystemType,
		},
		Config: configContract{
			Path:                          resolved.Config.Path,
			Source:                        configSource(resolved.Config),
			EntrypointPath:                resolved.Definition.EntrypointPath(contextData),
			TasksRoot:                     resolved.Config.Tasks.Root,
			ThreadsDefaultProjectStrategy: resolved.Config.Threads.DefaultProjectStrategy,
			ThreadsWorkspaceRoots:         resolved.Config.Threads.WorkspaceRoots,
		},
		Context:        context,
		PromptContract: renderPromptContract(contextData.ExecutionTarget, context),
		Checks:         checks,
		Summary:        summarizeChecks(checks),
	}

	return data, nil
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

func detectFilesystemType(path string) string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("stat", "-f", "%T", path)
	case "linux":
		cmd = exec.Command("stat", "-f", "-c", "%T", path)
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func checkWorkingDirectoryWritable(wd string) check {
	file, err := os.CreateTemp(wd, ".anton-doctor-*")
	if err != nil {
		return check{
			Name:   "working-directory-writable",
			Status: statusBlocked,
			Detail: fmt.Sprintf("cannot create temp file in %s: %v", wd, err),
			Hint:   "fix directory permissions or run Anton from a writable path",
		}
	}

	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)

	return check{
		Name:   "working-directory-writable",
		Status: statusOK,
		Detail: fmt.Sprintf("created and removed a temp file in %s", wd),
	}
}

func checkRepositoryContext(context adapter.Context) check {
	detail := context.WorkspaceKind
	if context.RepositoryRoot != "" {
		detail = fmt.Sprintf("%s (%s)", context.WorkspaceKind, context.RepositoryRoot)
	}
	if context.GitBranch != "" {
		detail = fmt.Sprintf("%s on branch %s", detail, context.GitBranch)
	}

	return check{
		Name:   "repository-context",
		Status: statusOK,
		Detail: detail,
	}
}

func checkFilesystem(filesystemType string) check {
	if filesystemType == "" {
		return check{
			Name:   "filesystem-type",
			Status: statusDegraded,
			Detail: "filesystem type could not be determined",
			Hint:   "verify mount risk manually on this host if builds behave differently from local runs",
		}
	}

	risky := map[string]bool{
		"fuse":    true,
		"fuseblk": true,
		"nfs":     true,
		"smbfs":   true,
	}
	if risky[filesystemType] {
		return check{
			Name:   "filesystem-type",
			Status: statusDegraded,
			Detail: fmt.Sprintf("filesystem %s is known to cause host-specific toolchain drift", filesystemType),
			Hint:   "prefer local build/output directories or absolute binary paths on this mount",
		}
	}

	return check{
		Name:   "filesystem-type",
		Status: statusOK,
		Detail: fmt.Sprintf("filesystem %s looks safe for normal CLI execution", filesystemType),
	}
}

func checkAntonConfig(config adapter.Config) check {
	if config.Loaded {
		return check{
			Name:   "anton-config",
			Status: statusOK,
			Detail: fmt.Sprintf("loaded repo-local anton.yaml from %s", config.Path),
		}
	}

	return check{
		Name:   "anton-config",
		Status: statusDegraded,
		Detail: fmt.Sprintf("no anton.yaml found at %s; using built-in defaults", config.Path),
		Hint:   "add a repo-local anton.yaml so the repo declares its Anton contract explicitly",
	}
}

func checkEntrypointFile(path string) check {
	if _, err := os.Stat(path); err == nil {
		return check{
			Name:   "entrypoint-file",
			Status: statusOK,
			Detail: fmt.Sprintf("found configured entrypoint at %s", path),
		}
	}

	return check{
		Name:   "entrypoint-file",
		Status: statusDegraded,
		Detail: fmt.Sprintf("configured entrypoint is missing at %s", path),
		Hint:   "create the configured repo-local entrypoint so prompts can consume one stable contract",
	}
}

func checkCodexThreads(envMap map[string]string) check {
	if path, err := exec.LookPath("codex-threads"); err == nil {
		return check{
			Name:   "codex-threads",
			Status: statusOK,
			Detail: fmt.Sprintf("found codex-threads on PATH at %s", path),
		}
	}

	home := envMap["HOME"]
	if home != "" {
		fallback := filepath.Join(home, ".local", "bin", "codex-threads")
		if _, err := os.Stat(fallback); err == nil {
			return check{
				Name:   "codex-threads",
				Status: statusDegraded,
				Detail: fmt.Sprintf("codex-threads exists at %s but is not on PATH", fallback),
				Hint:   "add ~/.local/bin to PATH or use the absolute binary path in remote shells",
			}
		}
	}

	return check{
		Name:   "codex-threads",
		Status: statusDegraded,
		Detail: "codex-threads is not available on PATH",
		Hint:   "install codex-threads before using anton threads, or skip threads workflows on this host",
	}
}

func configSource(config adapter.Config) string {
	if config.Loaded {
		return "repo-local anton.yaml"
	}
	return "built-in defaults"
}

func summarizeChecks(checks []check) summary {
	result := summary{Status: statusOK}
	for _, item := range checks {
		switch item.Status {
		case statusOK:
			result.OKCount++
		case statusDegraded:
			result.DegradedCount++
			if result.Status != statusBlocked {
				result.Status = statusDegraded
			}
		case statusBlocked:
			result.BlockedCount++
			result.Status = statusBlocked
		}
	}
	return result
}

func renderPromptContract(executionTarget string, context contextContract) string {
	lines := []string{
		fmt.Sprintf("Execution target: %s", executionTarget),
		fmt.Sprintf("Working directory: %s", context.WorkingDirectory),
		fmt.Sprintf("Workspace kind: %s", context.WorkspaceKind),
	}

	if context.RepositoryRoot != "" {
		lines = append(lines, fmt.Sprintf("Repository root: %s", context.RepositoryRoot))
	}
	if context.RepositoryKind != "" {
		lines = append(lines, fmt.Sprintf("Repository kind: %s", context.RepositoryKind))
	}
	if context.GitBranch != "" {
		lines = append(lines, fmt.Sprintf("Git branch: %s", context.GitBranch))
	}
	if len(context.ScopePaths) > 0 {
		lines = append(lines, fmt.Sprintf("Scope paths: %s", strings.Join(context.ScopePaths, ", ")))
	}

	return strings.Join(lines, "\n")
}

func renderHuman(stdout io.Writer, output report) {
	if output.Data == nil {
		return
	}

	data := output.Data
	_, _ = fmt.Fprintf(stdout, "Anton doctor\n")
	_, _ = fmt.Fprintf(stdout, "Status: %s\n\n", data.Summary.Status)

	_, _ = fmt.Fprintf(stdout, "Execution\n")
	_, _ = fmt.Fprintf(stdout, "  Adapter: %s\n", data.Adapter)
	_, _ = fmt.Fprintf(stdout, "  Target: %s\n", data.Environment.ExecutionTarget)
	_, _ = fmt.Fprintf(stdout, "  Host: %s\n", data.Environment.Host)
	_, _ = fmt.Fprintf(stdout, "  OS/Arch: %s/%s\n", data.Environment.OperatingSystem, data.Environment.Architecture)
	if data.Environment.FilesystemType != "" {
		_, _ = fmt.Fprintf(stdout, "  Filesystem: %s\n", data.Environment.FilesystemType)
	}
	_, _ = fmt.Fprintf(stdout, "  Working dir: %s\n", data.Context.WorkingDirectory)
	_, _ = fmt.Fprintf(stdout, "  Workspace kind: %s\n", data.Context.WorkspaceKind)
	if data.Context.RepositoryRoot != "" {
		_, _ = fmt.Fprintf(stdout, "  Repo root: %s\n", data.Context.RepositoryRoot)
	}
	if data.Context.GitBranch != "" {
		_, _ = fmt.Fprintf(stdout, "  Branch: %s\n", data.Context.GitBranch)
	}

	_, _ = fmt.Fprintf(stdout, "\nConfig\n")
	_, _ = fmt.Fprintf(stdout, "  Source: %s\n", data.Config.Source)
	_, _ = fmt.Fprintf(stdout, "  Path: %s\n", data.Config.Path)
	_, _ = fmt.Fprintf(stdout, "  Entrypoint: %s\n", data.Config.EntrypointPath)
	_, _ = fmt.Fprintf(stdout, "  Tasks root: %s\n", data.Config.TasksRoot)
	_, _ = fmt.Fprintf(stdout, "  Threads strategy: %s\n", data.Config.ThreadsDefaultProjectStrategy)
	if len(data.Config.ThreadsWorkspaceRoots) > 0 {
		_, _ = fmt.Fprintf(stdout, "  Workspace roots: %s\n", strings.Join(data.Config.ThreadsWorkspaceRoots, ", "))
	}

	_, _ = fmt.Fprintf(stdout, "\nChecks\n")
	for _, item := range data.Checks {
		_, _ = fmt.Fprintf(stdout, "  %-8s %s: %s\n", strings.ToUpper(item.Status), item.Name, item.Detail)
		if item.Hint != "" {
			_, _ = fmt.Fprintf(stdout, "           hint: %s\n", item.Hint)
		}
	}

	_, _ = fmt.Fprintf(stdout, "\nPrompt Contract\n%s\n", data.PromptContract)
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	if asJSON {
		payload := report{
			OK:      false,
			Command: command,
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
