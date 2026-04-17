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
	"gopkg.in/yaml.v3"
)

const (
	statusOK      = "ok"
	statusBlocked = "blocked"
)

var finishStates = map[string]bool{
	"active":  true,
	"blocked": true,
	"review":  true,
	"partial": true,
	"done":    true,
}

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

type lifecycleContract struct {
	Lifecycle                string `json:"lifecycle,omitempty"`
	FinishState              string `json:"finish_state,omitempty"`
	NextStep                 string `json:"next_step,omitempty"`
	BlockerCount             int    `json:"blocker_count,omitempty"`
	ExpectedDeliverableCount int    `json:"expected_deliverable_count,omitempty"`
}

type evidenceContract struct {
	AttemptCount    int `json:"attempt_count,omitempty"`
	ValidationCount int `json:"validation_count,omitempty"`
}

type commandData struct {
	Adapter          string            `json:"adapter"`
	WorkingDirectory string            `json:"working_directory"`
	Config           configContract    `json:"config"`
	BundleRoot       string            `json:"bundle_root"`
	StatusPath       string            `json:"status_path"`
	TaskID           string            `json:"task_id,omitempty"`
	Lifecycle        lifecycleContract `json:"lifecycle,omitempty"`
	Evidence         evidenceContract  `json:"evidence,omitempty"`
	Files            []fileResult      `json:"files"`
	Summary          summary           `json:"summary"`
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

type closeOptions struct {
	options
	State        string
	NextStep     string
	Blockers     []string
	Deliverables []string
	Artifacts    []string
}

type retargetOptions struct {
	options
	TaskID string
}

type importOptions struct {
	options
	From string
	Mode string
}

type taskStatus struct {
	Version int `yaml:"version"`
	Stable  struct {
		TaskID    string `yaml:"task_id"`
		CreatedAt string `yaml:"created_at"`
	} `yaml:"stable"`
	State struct {
		Lifecycle string `yaml:"lifecycle"`
		UpdatedAt string `yaml:"updated_at"`
	} `yaml:"state"`
	Machine struct {
		Host             string `yaml:"host"`
		ExecutionTarget  string `yaml:"execution_target"`
		WorkingDirectory string `yaml:"working_directory"`
		WorkspaceKind    string `yaml:"workspace_kind"`
	} `yaml:"machine"`
	Evidence struct {
		Attempts    []taskEvidence `yaml:"attempts"`
		Validations []taskEvidence `yaml:"validations"`
	} `yaml:"evidence"`
	Closure struct {
		FinishState          string   `yaml:"finish_state"`
		NextStep             string   `yaml:"next_step"`
		Blockers             []string `yaml:"blockers"`
		ExpectedDeliverables []string `yaml:"expected_deliverables"`
	} `yaml:"closure"`
}

type taskEvidence struct {
	Command   string   `yaml:"command" json:"command"`
	At        string   `yaml:"at" json:"at"`
	Outcome   string   `yaml:"outcome" json:"outcome"`
	Validated bool     `yaml:"validated" json:"validated"`
	Artifacts []string `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
	Notes     string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

type importedEvidence struct {
	Attempts    []taskEvidence `yaml:"attempts" json:"attempts"`
	Validations []taskEvidence `yaml:"validations" json:"validations"`
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
	case "close":
		return runClose(args[1:], stdout, stderr, environ)
	case "reopen":
		return runReopen(args[1:], stdout, stderr, environ)
	case "retarget":
		return runRetarget(args[1:], stdout, stderr, environ)
	case "import":
		return runImport(args[1:], stdout, stderr, environ)
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
	lifecycle := lifecycleContract{}
	evidence := evidenceContract{}
	if existing, statErr := os.Stat(statusPath); statErr == nil {
		if existing.IsDir() {
			return writeError("task-state init", "task-state-init-failed", fmt.Sprintf("%s is a directory, expected a file", statusPath), opts.JSON, stdout, stderr, 1)
		}

		snapshot, loadErr := resolved.Definition.ReadStatus(statusPath)
		if loadErr != nil {
			return writeError("task-state init", "task-state-init-failed", loadErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		taskID = snapshot.TaskID
		lifecycle = lifecycleFromSnapshot(snapshot)
		evidence = evidenceFromSnapshot(snapshot)
		files = append(files, fileResult{
			Path:   statusPath,
			Status: "existing",
			Detail: "kept existing status.yaml",
		})
	} else if os.IsNotExist(statErr) {
		content, snapshot, initErr := resolved.Definition.InitStatus(context, bundle, now)
		if initErr != nil {
			return writeError("task-state init", "task-state-init-failed", initErr.Error(), opts.JSON, stdout, stderr, 1)
		}
		taskID = snapshot.TaskID
		lifecycle = lifecycleFromSnapshot(snapshot)
		evidence = evidenceFromSnapshot(snapshot)
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
	} else if statErr != nil {
		return writeError("task-state init", "task-state-init-failed", statErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           taskID,
		Lifecycle:        lifecycle,
		Evidence:         evidence,
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
		Lifecycle:        lifecycleFromSnapshot(updatedSnapshot),
		Evidence:         evidenceFromSnapshot(updatedSnapshot),
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
	lifecycle := lifecycleContract{}
	evidence := evidenceContract{}
	if _, statErr := os.Stat(statusPath); statErr == nil {
		snapshot, statusErr := resolved.Definition.ReadStatus(statusPath)
		if statusErr != nil {
			files = append(files, fileResult{
				Path:   statusPath,
				Status: "invalid",
				Detail: statusErr.Error(),
			})
		} else {
			taskID = snapshot.TaskID
			lifecycle = lifecycleFromSnapshot(snapshot)
			evidence = evidenceFromSnapshot(snapshot)
			files = append(files, fileResult{
				Path:   statusPath,
				Status: "existing",
				Detail: "status.yaml schema looks valid",
			})
			files = append(files, closureGateResults(statusPath, snapshot)...)
		}
	} else if os.IsNotExist(statErr) {
		files = append(files, fileResult{
			Path:   statusPath,
			Status: "missing",
			Detail: "run `anton task-state init` to create status.yaml",
		})
	} else {
		return writeError("task-state check", "task-state-check-failed", statErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           taskID,
		Lifecycle:        lifecycle,
		Evidence:         evidence,
		Files:            files,
		Summary:          summarize(files),
	}

	exitCode := 0
	if data.Summary.Status == statusBlocked {
		exitCode = 1
	}
	return writeResponseWithExit("task-state check", data, opts.JSON, stdout, exitCode)
}

func runClose(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseCloseOptions(args)
	if err != nil {
		return writeError("task-state close", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, resolved, bundle, statusPath, failCode := resolveRuntime("task-state close", stdout, stderr, environ, opts.options)
	if failCode != 0 {
		return failCode
	}

	status, err := readTaskStatus(statusPath)
	if err != nil {
		return writeError("task-state close", "task-state-close-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	status.State.Lifecycle = opts.State
	status.State.UpdatedAt = now
	status.Closure.FinishState = opts.State
	if strings.TrimSpace(opts.NextStep) != "" {
		status.Closure.NextStep = opts.NextStep
	}
	if strings.TrimSpace(status.Closure.NextStep) == "" {
		status.Closure.NextStep = "share a handoff pack and request review feedback"
	}
	status.Closure.Blockers = append([]string{}, opts.Blockers...)
	if len(opts.Deliverables) > 0 {
		status.Closure.ExpectedDeliverables = append([]string{}, opts.Deliverables...)
	}
	status.Evidence.Attempts = append(status.Evidence.Attempts, taskEvidence{
		Command:   "anton task-state close",
		At:        now,
		Outcome:   fmt.Sprintf("set lifecycle to %s", opts.State),
		Validated: false,
	})
	for _, artifact := range opts.Artifacts {
		status.Evidence.Validations = append(status.Evidence.Validations, taskEvidence{
			Command:   "anton task-state close",
			At:        now,
			Outcome:   "validated artifact for close gate",
			Validated: true,
			Artifacts: []string{artifact},
		})
	}

	violations := closureGateViolations(status)
	if len(violations) > 0 {
		return writeError("task-state close", "task-state-close-failed", strings.Join(violations, "; "), opts.JSON, stdout, stderr, 1)
	}

	if err := writeTaskStatus(statusPath, status); err != nil {
		return writeError("task-state close", "task-state-close-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	snapshot, readErr := resolved.Definition.ReadStatus(statusPath)
	if readErr != nil {
		return writeError("task-state close", "task-state-close-failed", readErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	files := validateBundle(bundle.Root, bundle)
	files = append(files, fileResult{
		Path:   statusPath,
		Status: "updated",
		Detail: fmt.Sprintf("updated lifecycle to %s and evaluated closure gates", opts.State),
	})

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           snapshot.TaskID,
		Lifecycle:        lifecycleFromSnapshot(snapshot),
		Evidence:         evidenceFromSnapshot(snapshot),
		Files:            files,
		Summary:          summarize(files),
	}
	exitCode := 0
	if data.Summary.Status == statusBlocked {
		exitCode = 1
	}
	return writeResponseWithExit("task-state close", data, opts.JSON, stdout, exitCode)
}

func runReopen(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("task-state reopen", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, resolved, bundle, statusPath, failCode := resolveRuntime("task-state reopen", stdout, stderr, environ, opts)
	if failCode != 0 {
		return failCode
	}

	status, err := readTaskStatus(statusPath)
	if err != nil {
		return writeError("task-state reopen", "task-state-reopen-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	status.State.Lifecycle = "active"
	status.State.UpdatedAt = now
	status.Closure.FinishState = "active"
	status.Closure.Blockers = []string{}
	status.Evidence.Attempts = append(status.Evidence.Attempts, taskEvidence{
		Command:   "anton task-state reopen",
		At:        now,
		Outcome:   "reopened lifecycle to active",
		Validated: false,
	})

	if err := writeTaskStatus(statusPath, status); err != nil {
		return writeError("task-state reopen", "task-state-reopen-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	snapshot, readErr := resolved.Definition.ReadStatus(statusPath)
	if readErr != nil {
		return writeError("task-state reopen", "task-state-reopen-failed", readErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	files := validateBundle(bundle.Root, bundle)
	files = append(files, fileResult{
		Path:   statusPath,
		Status: "updated",
		Detail: "reopened lifecycle to active",
	})

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           snapshot.TaskID,
		Lifecycle:        lifecycleFromSnapshot(snapshot),
		Evidence:         evidenceFromSnapshot(snapshot),
		Files:            files,
		Summary:          summarize(files),
	}
	return writeResponse("task-state reopen", data, opts.JSON, stdout)
}

func runRetarget(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseRetargetOptions(args)
	if err != nil {
		return writeError("task-state retarget", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, resolved, bundle, statusPath, failCode := resolveRuntime("task-state retarget", stdout, stderr, environ, opts.options)
	if failCode != 0 {
		return failCode
	}

	newRoot := filepath.Join(filepath.Dir(bundle.Root), opts.TaskID)
	if filepath.Clean(newRoot) != filepath.Clean(bundle.Root) {
		if _, err := os.Stat(newRoot); err == nil {
			return writeError("task-state retarget", "task-state-retarget-failed", fmt.Sprintf("target bundle already exists: %s", newRoot), opts.JSON, stdout, stderr, 1)
		}
		if err := os.MkdirAll(filepath.Dir(newRoot), 0o755); err != nil {
			return writeError("task-state retarget", "task-state-retarget-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		}
		if err := os.Rename(bundle.Root, newRoot); err != nil {
			return writeError("task-state retarget", "task-state-retarget-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		}
	}

	statusPath = filepath.Join(newRoot, "status.yaml")
	status, err := readTaskStatus(statusPath)
	if err != nil {
		return writeError("task-state retarget", "task-state-retarget-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	status.Stable.TaskID = opts.TaskID
	status.State.UpdatedAt = now
	status.Evidence.Attempts = append(status.Evidence.Attempts, taskEvidence{
		Command:   "anton task-state retarget",
		At:        now,
		Outcome:   "retargeted task bundle id",
		Validated: false,
		Notes:     fmt.Sprintf("bundle moved to %s", newRoot),
	})
	if err := writeTaskStatus(statusPath, status); err != nil {
		return writeError("task-state retarget", "task-state-retarget-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	snapshot, readErr := resolved.Definition.ReadStatus(statusPath)
	if readErr != nil {
		return writeError("task-state retarget", "task-state-retarget-failed", readErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	files := []fileResult{
		{
			Path:   bundle.Root,
			Status: "updated",
			Detail: fmt.Sprintf("retargeted bundle root to %s", newRoot),
		},
		{
			Path:   statusPath,
			Status: "updated",
			Detail: fmt.Sprintf("updated stable.task_id to %s", opts.TaskID),
		},
	}

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       newRoot,
		StatusPath:       statusPath,
		TaskID:           snapshot.TaskID,
		Lifecycle:        lifecycleFromSnapshot(snapshot),
		Evidence:         evidenceFromSnapshot(snapshot),
		Files:            files,
		Summary:          summarize(files),
	}
	return writeResponse("task-state retarget", data, opts.JSON, stdout)
}

func runImport(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseImportOptions(args)
	if err != nil {
		return writeError("task-state import", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, resolved, bundle, statusPath, failCode := resolveRuntime("task-state import", stdout, stderr, environ, opts.options)
	if failCode != 0 {
		return failCode
	}

	status, err := readTaskStatus(statusPath)
	if err != nil {
		return writeError("task-state import", "task-state-import-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	content, err := os.ReadFile(opts.From)
	if err != nil {
		return writeError("task-state import", "task-state-import-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	var imported importedEvidence
	if strings.HasSuffix(strings.ToLower(opts.From), ".json") {
		if err := json.Unmarshal(content, &imported); err != nil {
			return writeError("task-state import", "task-state-import-failed", fmt.Sprintf("parse %s: %v", opts.From, err), opts.JSON, stdout, stderr, 1)
		}
	} else {
		if err := yaml.Unmarshal(content, &imported); err != nil {
			return writeError("task-state import", "task-state-import-failed", fmt.Sprintf("parse %s: %v", opts.From, err), opts.JSON, stdout, stderr, 1)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if opts.Mode == "attempts" || opts.Mode == "both" {
		status.Evidence.Attempts = append(status.Evidence.Attempts, imported.Attempts...)
	}
	if opts.Mode == "validations" || opts.Mode == "both" {
		status.Evidence.Validations = append(status.Evidence.Validations, imported.Validations...)
	}
	status.State.UpdatedAt = now
	status.Evidence.Attempts = append(status.Evidence.Attempts, taskEvidence{
		Command:   "anton task-state import",
		At:        now,
		Outcome:   fmt.Sprintf("imported evidence from %s", opts.From),
		Validated: false,
	})

	if err := writeTaskStatus(statusPath, status); err != nil {
		return writeError("task-state import", "task-state-import-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	snapshot, readErr := resolved.Definition.ReadStatus(statusPath)
	if readErr != nil {
		return writeError("task-state import", "task-state-import-failed", readErr.Error(), opts.JSON, stdout, stderr, 1)
	}

	files := validateBundle(bundle.Root, bundle)
	files = append(files, fileResult{
		Path:   statusPath,
		Status: "updated",
		Detail: fmt.Sprintf("imported evidence from %s (%s)", opts.From, opts.Mode),
	})

	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           buildConfigContract(resolved.Config),
		BundleRoot:       bundle.Root,
		StatusPath:       statusPath,
		TaskID:           snapshot.TaskID,
		Lifecycle:        lifecycleFromSnapshot(snapshot),
		Evidence:         evidenceFromSnapshot(snapshot),
		Files:            files,
		Summary:          summarize(files),
	}
	return writeResponse("task-state import", data, opts.JSON, stdout)
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

func parseCloseOptions(args []string) (closeOptions, error) {
	opts := closeOptions{
		State: "review",
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--state":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --state")
			}
			state := strings.TrimSpace(args[index])
			if !finishStates[state] {
				return opts, fmt.Errorf("unsupported --state value %q", args[index])
			}
			opts.State = state
		case "--next-step":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --next-step")
			}
			opts.NextStep = strings.TrimSpace(args[index])
		case "--blocker":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --blocker")
			}
			value := strings.TrimSpace(args[index])
			if value != "" {
				opts.Blockers = append(opts.Blockers, value)
			}
		case "--deliverable":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --deliverable")
			}
			value := strings.TrimSpace(args[index])
			if value != "" {
				opts.Deliverables = append(opts.Deliverables, value)
			}
		case "--artifact":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --artifact")
			}
			value := strings.TrimSpace(args[index])
			if value != "" {
				opts.Artifacts = append(opts.Artifacts, value)
			}
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func parseRetargetOptions(args []string) (retargetOptions, error) {
	opts := retargetOptions{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--task-id":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --task-id")
			}
			opts.TaskID = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.TaskID == "" {
		return opts, fmt.Errorf("--task-id is required")
	}
	return opts, nil
}

func parseImportOptions(args []string) (importOptions, error) {
	opts := importOptions{Mode: "both"}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--from":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --from")
			}
			opts.From = strings.TrimSpace(args[index])
		case "--mode":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --mode")
			}
			mode := strings.TrimSpace(args[index])
			if mode != "attempts" && mode != "validations" && mode != "both" {
				return opts, fmt.Errorf("unsupported --mode value %q", args[index])
			}
			opts.Mode = mode
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.From == "" {
		return opts, fmt.Errorf("--from is required")
	}
	return opts, nil
}

func resolveRuntime(command string, stdout io.Writer, stderr io.Writer, environ []string, opts options) (string, adapter.Resolved, adapter.ResolvedTaskBundle, string, int) {
	wd, err := os.Getwd()
	if err != nil {
		_ = writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		return "", adapter.Resolved{}, adapter.ResolvedTaskBundle{}, "", 1
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		_ = writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		return "", adapter.Resolved{}, adapter.ResolvedTaskBundle{}, "", 1
	}
	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, time.Now().UTC())
	if err != nil {
		_ = writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		return "", adapter.Resolved{}, adapter.ResolvedTaskBundle{}, "", 1
	}
	return wd, resolved, bundle, bundle.StatusPath(), 0
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

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton task-state init [--json]
  anton task-state pulse [--json]
  anton task-state check [--json]
  anton task-state close [--json] [--state active|blocked|review|partial|done] [--next-step TEXT] [--blocker TEXT ...] [--deliverable TEXT ...] [--artifact PATH ...]
  anton task-state reopen [--json]
  anton task-state retarget --task-id ID [--json]
  anton task-state import --from PATH [--mode attempts|validations|both] [--json]
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
	if data.Lifecycle.Lifecycle != "" {
		_, _ = fmt.Fprintf(stdout, "Lifecycle: %s (%s)\n", data.Lifecycle.Lifecycle, data.Lifecycle.FinishState)
	}
	_, _ = fmt.Fprintf(stdout, "Evidence: attempts=%d validations=%d\n", data.Evidence.AttemptCount, data.Evidence.ValidationCount)
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

func lifecycleFromSnapshot(snapshot adapter.StatusSnapshot) lifecycleContract {
	return lifecycleContract{
		Lifecycle:                snapshot.Lifecycle,
		FinishState:              snapshot.FinishState,
		NextStep:                 snapshot.NextStep,
		BlockerCount:             snapshot.BlockerCount,
		ExpectedDeliverableCount: snapshot.ExpectedDeliverableCount,
	}
}

func evidenceFromSnapshot(snapshot adapter.StatusSnapshot) evidenceContract {
	return evidenceContract{
		AttemptCount:    snapshot.AttemptCount,
		ValidationCount: snapshot.ValidationCount,
	}
}

func readTaskStatus(path string) (taskStatus, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return taskStatus{}, fmt.Errorf("read %s: %w", path, err)
	}
	var status taskStatus
	if err := yaml.Unmarshal(content, &status); err != nil {
		return taskStatus{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return status, nil
}

func writeTaskStatus(path string, status taskStatus) error {
	content, err := yaml.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func closureGateViolations(status taskStatus) []string {
	violations := []string{}
	state := strings.TrimSpace(status.State.Lifecycle)
	if state == "" {
		return []string{"state.lifecycle is required"}
	}
	if !finishStates[state] {
		return []string{fmt.Sprintf("unsupported lifecycle %q", state)}
	}

	if state == "review" || state == "partial" || state == "done" || state == "blocked" {
		if strings.TrimSpace(status.Closure.NextStep) == "" {
			violations = append(violations, "closure.next_step is required for non-active lifecycle states")
		}
	}
	if state == "review" || state == "done" {
		if len(status.Closure.ExpectedDeliverables) == 0 {
			violations = append(violations, "closure.expected_deliverables must be non-empty for review/done states")
		}
	}
	if state == "done" {
		if len(status.Closure.Blockers) > 0 {
			violations = append(violations, "closure.blockers must be empty for done state")
		}
		if len(status.Evidence.Validations) == 0 {
			violations = append(violations, "evidence.validations must include at least one receipt for done state")
		}
	}

	return violations
}

func closureGateResults(statusPath string, snapshot adapter.StatusSnapshot) []fileResult {
	results := []fileResult{}
	if snapshot.Lifecycle == "" {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: "state.lifecycle is empty",
		})
		return results
	}
	if !finishStates[snapshot.Lifecycle] {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: fmt.Sprintf("unsupported state.lifecycle %q", snapshot.Lifecycle),
		})
		return results
	}
	if snapshot.Lifecycle != "active" && strings.TrimSpace(snapshot.NextStep) == "" {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: "closure.next_step is required for non-active lifecycle",
		})
	}
	if (snapshot.Lifecycle == "review" || snapshot.Lifecycle == "done") && snapshot.ExpectedDeliverableCount == 0 {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: "closure.expected_deliverables is required for review/done lifecycle",
		})
	}
	if snapshot.Lifecycle == "done" && snapshot.BlockerCount > 0 {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: "closure.blockers must be empty for done lifecycle",
		})
	}
	if snapshot.Lifecycle == "done" && snapshot.ValidationCount == 0 {
		results = append(results, fileResult{
			Path:   statusPath,
			Status: "invalid",
			Detail: "evidence.validations requires at least one receipt for done lifecycle",
		})
	}
	return results
}
