package taskstate

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

const (
	statusOK      = "ok"
	statusBlocked = "blocked"
)

type fileResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type summary struct {
	Status        string `json:"status"`
	CreatedCount  int    `json:"created_count"`
	ExistingCount int    `json:"existing_count"`
	UpdatedCount  int    `json:"updated_count"`
	MissingCount  int    `json:"missing_count"`
	InvalidCount  int    `json:"invalid_count"`
}

type configContract struct {
	Path      string `json:"path"`
	Source    string `json:"source"`
	TasksRoot string `json:"tasks_root"`
}

type commandData struct {
	Adapter          string         `json:"adapter"`
	WorkingDirectory string         `json:"working_directory"`
	Config           configContract `json:"config"`
	BundleRoot       string         `json:"bundle_root"`
	StatusPath       string         `json:"status_path"`
	TaskID           string         `json:"task_id,omitempty"`
	Files            []fileResult   `json:"files"`
	Summary          summary        `json:"summary"`
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

type options struct {
	JSON bool
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr, environ)
	case "pulse":
		return runPulse(args[1:], stdout, stderr, environ)
	case "check":
		return runCheck(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown task-state command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runInit(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("task-state init", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("task-state init", "task-state-init-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("task-state init", "task-state-init-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	now := time.Now().UTC()
	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, now)
	if err != nil {
		return writeError("task-state init", "task-state-init-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	context := resolved.Context
	statusPath := bundle.StatusPath()

	files := make([]fileResult, 0, len(bundle.RequiredFiles)+1)
	for _, file := range bundle.RequiredFiles {
		path := filepath.Join(bundle.Root, file.Name)
		result, createErr := ensureFile(path, file.Template)
		if createErr != nil {
			return writeError("task-state init", "task-state-init-failed", createErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		files = append(files, result)
	}

	taskID := ""
	if existing, err := os.Stat(statusPath); err == nil {
		if existing.IsDir() {
			return writeError("task-state init", "task-state-init-failed", fmt.Sprintf("%s is a directory, expected a file", statusPath), opts.JSON, stdout, stderr, 1)
		}

		snapshot, loadErr := resolved.Definition.ReadStatus(statusPath)
		if loadErr != nil {
			return writeError("task-state init", "task-state-init-failed", loadErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		taskID = snapshot.TaskID
		files = append(files, fileResult{
			Path:   statusPath,
			Status: "existing",
			Detail: "kept existing status.yaml",
		})
	} else if os.IsNotExist(err) {
		content, snapshot, initErr := resolved.Definition.InitStatus(context, bundle, now)
		if initErr != nil {
			return writeError("task-state init", "task-state-init-failed", initErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		taskID = snapshot.TaskID
		if mkdirErr := os.MkdirAll(filepath.Dir(statusPath), 0o755); mkdirErr != nil {
			return writeError("task-state init", "task-state-init-failed", mkdirErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		if writeErr := os.WriteFile(statusPath, content, 0o644); writeErr != nil {
			return writeError("task-state init", "task-state-init-failed", writeErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		files = append(files, fileResult{
			Path:   statusPath,
			Status: "created",
			Detail: "created status.yaml",
		})
	} else if err != nil {
		return writeError("task-state init", "task-state-init-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           taskID,
		Files:            files,
		Summary:          summarize(files),
	}
	return writeResponse("task-state init", data, opts.JSON, stdout)
}

func runPulse(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("task-state pulse", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, time.Now().UTC())
	if err != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	context := resolved.Context
	statusPath := bundle.StatusPath()
	snapshot, err := resolved.Definition.ReadStatus(statusPath)
	if err != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	content, updatedSnapshot, pulseErr := resolved.Definition.PulseStatus(statusPath, context, time.Now().UTC())
	if pulseErr != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", pulseErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	if err := os.WriteFile(statusPath, content, 0o644); err != nil {
		return writeError("task-state pulse", "task-state-pulse-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	files := validateBundle(bundle.Root, bundle)
	files = append(files, fileResult{
		Path:   statusPath,
		Status: "updated",
		Detail: "updated status.yaml machine metadata and timestamp",
	})

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           chooseTaskID(updatedSnapshot.TaskID, snapshot.TaskID),
		Files:            files,
		Summary:          summarize(files),
	}

	exitCode := 0
	if data.Summary.Status == statusBlocked {
		exitCode = 1
	}
	return writeResponseWithExit("task-state pulse", data, opts.JSON, stdout, exitCode)
}

func runCheck(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("task-state check", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("task-state check", "task-state-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("task-state check", "task-state-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, time.Now().UTC())
	if err != nil {
		return writeError("task-state check", "task-state-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	files := validateBundle(bundle.Root, bundle)
	statusPath := bundle.StatusPath()

	taskID := ""
	if _, err := os.Stat(statusPath); err == nil {
		snapshot, statusErr := resolved.Definition.ReadStatus(statusPath)
		if statusErr != nil {
			files = append(files, fileResult{
				Path:   statusPath,
				Status: "invalid",
				Detail: statusErr.Error(),
			})
		} else {
			taskID = snapshot.TaskID
			files = append(files, fileResult{
				Path:   statusPath,
				Status: "existing",
				Detail: "status.yaml schema looks valid",
			})
		}
	} else if os.IsNotExist(err) {
		files = append(files, fileResult{
			Path:   statusPath,
			Status: "missing",
			Detail: "run `anton task-state init` to create status.yaml",
		})
	} else {
		return writeError("task-state check", "task-state-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           taskID,
		Files:            files,
		Summary:          summarize(files),
	}

	exitCode := 0
	if data.Summary.Status == statusBlocked {
		exitCode = 1
	}
	return writeResponseWithExit("task-state check", data, opts.JSON, stdout, exitCode)
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

func ensureFile(path string, template string) (fileResult, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fileResult{}, fmt.Errorf("%s is a directory, expected a file", path)
		}
		return fileResult{
			Path:   path,
			Status: "existing",
			Detail: "kept existing file",
		}, nil
	}
	if !os.IsNotExist(err) {
		return fileResult{}, err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
		return fileResult{}, mkdirErr
	}
	if writeErr := os.WriteFile(path, []byte(template), 0o644); writeErr != nil {
		return fileResult{}, writeErr
	}

	return fileResult{
		Path:   path,
		Status: "created",
		Detail: "created file from Anton template",
	}, nil
}

func validateBundle(bundleRoot string, bundle adapter.ResolvedTaskBundle) []fileResult {
	results := make([]fileResult, 0, len(bundle.RequiredFiles))
	for _, file := range bundle.RequiredFiles {
		path := filepath.Join(bundleRoot, file.Name)
		info, err := os.Stat(path)
		switch {
		case err == nil && !info.IsDir():
			results = append(results, fileResult{
				Path:   path,
				Status: "existing",
				Detail: "required file is present",
			})
		case err == nil && info.IsDir():
			results = append(results, fileResult{
				Path:   path,
				Status: "invalid",
				Detail: "expected a file but found a directory",
			})
		case os.IsNotExist(err):
			results = append(results, fileResult{
				Path:   path,
				Status: "missing",
				Detail: "required file is missing",
			})
		default:
			results = append(results, fileResult{
				Path:   path,
				Status: "invalid",
				Detail: err.Error(),
			})
		}
	}
	return results
}

func summarize(files []fileResult) summary {
	result := summary{Status: statusOK}
	for _, file := range files {
		switch file.Status {
		case "created":
			result.CreatedCount++
		case "existing":
			result.ExistingCount++
		case "updated":
			result.UpdatedCount++
		case "missing":
			result.MissingCount++
			result.Status = statusBlocked
		case "invalid":
			result.InvalidCount++
			result.Status = statusBlocked
		}
	}
	return result
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton task-state init [--json]
  anton task-state pulse [--json]
  anton task-state check [--json]
`
}

func writeResponse(command string, data commandData, asJSON bool, stdout io.Writer) int {
	return writeResponseWithExit(command, data, asJSON, stdout, 0)
}

func writeResponseWithExit(command string, data commandData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{
		OK:      data.Summary.Status == statusOK,
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
	_, _ = fmt.Fprintf(stdout, "Adapter: %s\n", data.Adapter)
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", data.Summary.Status)
	_, _ = fmt.Fprintf(stdout, "Config source: %s\n", data.Config.Source)
	if data.TaskID != "" {
		_, _ = fmt.Fprintf(stdout, "Task ID: %s\n", data.TaskID)
	}
	_, _ = fmt.Fprintf(stdout, "Working dir: %s\n\n", data.WorkingDirectory)
	_, _ = fmt.Fprintf(stdout, "Bundle root: %s\n\n", data.BundleRoot)
	_, _ = fmt.Fprintf(stdout, "Files\n")
	for _, file := range data.Files {
		_, _ = fmt.Fprintf(stdout, "  %-8s %s: %s\n", strings.ToUpper(file.Status), file.Path, file.Detail)
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

func chooseTaskID(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildConfigContract(config adapter.Config) configContract {
	return configContract{
		Path:      config.Path,
		Source:    config.Source(),
		TasksRoot: config.Tasks.Root,
	}
}
