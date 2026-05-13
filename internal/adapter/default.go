package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var genericTaskBranchPattern = regexp.MustCompile(`^task(?:/[^/]+)*/([^/]+)$`)

var ErrTaskIdentityRequired = errors.New("task identity required")

type TaskIdentityRequiredError struct {
	TasksRoot string
}

func (err TaskIdentityRequiredError) Error() string {
	return fmt.Sprintf("task identity required; set ANTON_TASK_ID, use a task/<id_slug> branch, or run inside an existing %s bundle", filepath.ToSlash(err.TasksRoot))
}

func (err TaskIdentityRequiredError) Unwrap() error {
	return ErrTaskIdentityRequired
}

type Default struct {
	Config Config
}

func (Default) Name() string {
	return "canonical"
}

func (definition Default) TaskBundle(context Context, environ []string, now time.Time) (ResolvedTaskBundle, error) {
	tasksRoot := definition.tasksRoot(context)
	if definition.taskLayout() == "topic-layer" {
		return definition.topicLayerTaskBundle(context, environ, now, tasksRoot)
	}

	if current := currentAntonTaskBundleRoot(context.WorkingDirectory, tasksRoot); current != "" {
		if err := ValidateTaskID(filepath.Base(current)); err != nil {
			return ResolvedTaskBundle{}, fmt.Errorf("current canonical task bundle root has invalid task id: %w", err)
		}
		return ResolvedTaskBundle{
			Root:          current,
			RequiredFiles: defaultTaskFiles(now),
			StatusFile:    "status.yaml",
		}, nil
	}

	taskID := inferTaskID(context, environ)
	if trimString(taskID) == "" {
		return ResolvedTaskBundle{}, TaskIdentityRequiredError{TasksRoot: tasksRoot}
	}
	if err := ValidateTaskID(taskID); err != nil {
		return ResolvedTaskBundle{}, fmt.Errorf("canonical task bundle root inferred invalid task id %q: %w", taskID, err)
	}

	return ResolvedTaskBundle{
		Root:          filepath.Join(tasksRoot, "active", taskID),
		RequiredFiles: defaultTaskFiles(now),
		StatusFile:    "status.yaml",
	}, nil
}

func (definition Default) EntrypointPath(context Context) string {
	return definition.resolvePath(context, definition.effectiveConfig().Entrypoint.Path)
}

type defaultStatus struct {
	Version  int                  `yaml:"version"`
	Stable   defaultStableSection `yaml:"stable"`
	State    defaultStateSection  `yaml:"state"`
	Machine  defaultMachine       `yaml:"machine"`
	Evidence defaultEvidence      `yaml:"evidence"`
	Closure  defaultClosure       `yaml:"closure"`
	Extra    map[string]any       `yaml:",inline"`
}

type defaultStableSection struct {
	TaskID    string         `yaml:"task_id"`
	CreatedAt string         `yaml:"created_at"`
	Extra     map[string]any `yaml:",inline"`
}

type defaultStateSection struct {
	Lifecycle string         `yaml:"lifecycle"`
	UpdatedAt string         `yaml:"updated_at"`
	Extra     map[string]any `yaml:",inline"`
}

type defaultEvidence struct {
	Attempts    []defaultEvidenceReceipt `yaml:"attempts"`
	Validations []defaultEvidenceReceipt `yaml:"validations"`
	Extra       map[string]any           `yaml:",inline"`
}

type defaultEvidenceReceipt struct {
	Command   string   `yaml:"command"`
	At        string   `yaml:"at"`
	Outcome   string   `yaml:"outcome"`
	Validated bool     `yaml:"validated"`
	Artifacts []string `yaml:"artifacts,omitempty"`
	Notes     string   `yaml:"notes,omitempty"`
}

type defaultClosure struct {
	FinishState          string         `yaml:"finish_state"`
	NextStep             string         `yaml:"next_step"`
	Blockers             []string       `yaml:"blockers"`
	ExpectedDeliverables []string       `yaml:"expected_deliverables"`
	Extra                map[string]any `yaml:",inline"`
}

type defaultMachine struct {
	Host             string         `yaml:"host"`
	ExecutionTarget  string         `yaml:"execution_target"`
	WorkingDirectory string         `yaml:"working_directory"`
	WorkspaceKind    string         `yaml:"workspace_kind"`
	Extra            map[string]any `yaml:",inline"`
}

type physEditStatus struct {
	Version     int                 `yaml:"version"`
	Task        physEditTask        `yaml:"task"`
	Execution   physEditExecution   `yaml:"execution"`
	Environment physEditEnvironment `yaml:"environment"`
	Services    []physEditService   `yaml:"services"`
	State       physEditState       `yaml:"state"`
	Freshness   map[string]any      `yaml:"freshness,omitempty"`
	Extra       map[string]any      `yaml:",inline"`
}

type physEditTask struct {
	ID          string         `yaml:"id"`
	Topic       string         `yaml:"topic"`
	Title       string         `yaml:"title"`
	Lifecycle   string         `yaml:"lifecycle"`
	Phase       string         `yaml:"phase"`
	Owner       string         `yaml:"owner"`
	LastUpdated string         `yaml:"last_updated"`
	Extra       map[string]any `yaml:",inline"`
}

type physEditExecution struct {
	Mode       string         `yaml:"mode,omitempty"`
	Checkout   string         `yaml:"checkout,omitempty"`
	Worktree   string         `yaml:"worktree"`
	Branch     string         `yaml:"branch"`
	CWD        string         `yaml:"cwd"`
	ScopePaths []string       `yaml:"scope_paths"`
	BatchRJob  bool           `yaml:"batch_rjob"`
	Extra      map[string]any `yaml:",inline"`
}

type physEditEnvironment struct {
	MachineType string         `yaml:"machine_type"`
	Host        string         `yaml:"host"`
	TmuxSession string         `yaml:"tmux_session"`
	TmuxWindow  string         `yaml:"tmux_window"`
	Proxy       string         `yaml:"proxy"`
	CondaEnv    string         `yaml:"conda_env"`
	Python      string         `yaml:"python"`
	Notes       string         `yaml:"notes"`
	Extra       map[string]any `yaml:",inline"`
}

type physEditService struct {
	Name       string         `yaml:"name"`
	Kind       string         `yaml:"kind"`
	Status     string         `yaml:"status"`
	Worktree   string         `yaml:"worktree"`
	ReopenHint string         `yaml:"reopen_hint"`
	Extra      map[string]any `yaml:",inline"`
}

type physEditState struct {
	Done       []string       `yaml:"done"`
	InProgress []string       `yaml:"in_progress"`
	Blockers   []string       `yaml:"blockers"`
	NextAction []string       `yaml:"next_actions"`
	NeedsUser  []string       `yaml:"needs_user"`
	Extra      map[string]any `yaml:",inline"`
}

func (definition Default) ReadStatus(path string) (StatusSnapshot, error) {
	return definition.ReadStatusWithSchema(path, "auto")
}

func (definition Default) ReadStatusWithSchema(path string, schema string) (StatusSnapshot, error) {
	if schema == "" || schema == "auto" {
		schema = definition.statusSchema()
	}
	if schema == "physedit-v1" {
		return readPhysEditStatus(path)
	}
	if schema != "anton" {
		return StatusSnapshot{}, fmt.Errorf("unsupported status schema %q", schema)
	}

	status := defaultStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return StatusSnapshot{}, err
	}

	if err := validateDefaultStatus(path, status); err != nil {
		return StatusSnapshot{}, err
	}

	return snapshotFromDefaultStatus(status), nil
}

func (definition Default) InitStatus(context Context, bundle ResolvedTaskBundle, now time.Time) ([]byte, StatusSnapshot, error) {
	if definition.statusSchema() == "physedit-v1" {
		return initPhysEditStatus(context, bundle, now)
	}
	return definition.initAntonStatus(context, bundle, now)
}

func (definition Default) initAntonStatus(context Context, bundle ResolvedTaskBundle, now time.Time) ([]byte, StatusSnapshot, error) {
	taskID := filepath.Base(bundle.Root)
	status := defaultStatus{
		Version: 1,
		Stable: defaultStableSection{
			TaskID:    taskID,
			CreatedAt: now.UTC().Format(time.RFC3339),
		},
		State: defaultStateSection{
			Lifecycle: "active",
			UpdatedAt: now.UTC().Format(time.RFC3339),
		},
		Machine: defaultMachine{
			Host:             fallbackString(context.Host, "unknown"),
			ExecutionTarget:  context.ExecutionTarget,
			WorkingDirectory: context.WorkingDirectory,
			WorkspaceKind:    context.WorkspaceKind,
		},
		Evidence: defaultEvidence{
			Attempts:    []defaultEvidenceReceipt{},
			Validations: []defaultEvidenceReceipt{},
		},
		Closure: defaultClosure{
			FinishState:          "active",
			NextStep:             "continue implementation and update progress.md",
			Blockers:             []string{},
			ExpectedDeliverables: []string{},
		},
	}

	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal default status: %w", err)
	}
	return content, snapshotFromDefaultStatus(status), nil
}

func initPhysEditStatus(context Context, bundle ResolvedTaskBundle, now time.Time) ([]byte, StatusSnapshot, error) {
	taskID := filepath.Base(bundle.Root)
	topic := topicFromBundleRoot(bundle.Root)
	worktree := context.RepositoryRoot
	if worktree == "" {
		worktree = context.WorkingDirectory
	}
	scopePaths := []string{}
	if context.RepositoryRoot != "" {
		if relative, err := filepath.Rel(context.RepositoryRoot, bundle.Root); err == nil && !strings.HasPrefix(relative, "..") {
			scopePaths = append(scopePaths, filepath.ToSlash(relative))
		}
	}
	status := physEditStatus{
		Version: 1,
		Task: physEditTask{
			ID:          taskID,
			Topic:       topic,
			Title:       taskID,
			Lifecycle:   "active",
			Phase:       "planning",
			Owner:       "unknown",
			LastUpdated: now.UTC().Format("2006-01-02"),
		},
		Execution: physEditExecution{
			Worktree:   worktree,
			Branch:     context.GitBranch,
			CWD:        context.WorkingDirectory,
			ScopePaths: scopePaths,
			BatchRJob:  false,
		},
		Environment: physEditEnvironment{
			MachineType: "unknown",
			Host:        fallbackString(context.Host, "unknown"),
			Proxy:       "unknown",
			Python:      "python3",
		},
		Services: []physEditService{},
		State: physEditState{
			Done:       []string{},
			InProgress: []string{},
			Blockers:   []string{},
			NextAction: []string{},
			NeedsUser:  []string{},
		},
	}
	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal physedit-v1 status: %w", err)
	}
	return content, snapshotFromPhysEditStatus(status), nil
}

func (definition Default) PulseStatus(path string, context Context, now time.Time) ([]byte, StatusSnapshot, error) {
	if definition.statusSchema() == "physedit-v1" {
		return pulsePhysEditStatus(path, context, now)
	}

	status := defaultStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return nil, StatusSnapshot{}, err
	}
	if err := validateDefaultStatus(path, status); err != nil {
		return nil, StatusSnapshot{}, err
	}

	status.State.UpdatedAt = now.UTC().Format(time.RFC3339)
	status.Machine.Host = fallbackString(context.Host, "unknown")
	status.Machine.ExecutionTarget = context.ExecutionTarget
	status.Machine.WorkingDirectory = context.WorkingDirectory
	status.Machine.WorkspaceKind = context.WorkspaceKind
	status.Evidence.Attempts = append(status.Evidence.Attempts, defaultEvidenceReceipt{
		Command:   "anton task-state pulse",
		At:        now.UTC().Format(time.RFC3339),
		Outcome:   "updated machine metadata and heartbeat timestamp",
		Validated: false,
	})
	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal default status: %w", err)
	}
	return content, snapshotFromDefaultStatus(status), nil
}

func pulsePhysEditStatus(path string, context Context, now time.Time) ([]byte, StatusSnapshot, error) {
	status := physEditStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return nil, StatusSnapshot{}, err
	}
	if err := validatePhysEditStatus(path, status); err != nil {
		return nil, StatusSnapshot{}, err
	}
	status.Task.LastUpdated = now.UTC().Format("2006-01-02")
	status.Execution.CWD = context.WorkingDirectory
	if context.GitBranch != "" {
		status.Execution.Branch = context.GitBranch
	}
	if context.RepositoryRoot != "" {
		status.Execution.Worktree = context.RepositoryRoot
	}
	status.Environment.Host = fallbackString(context.Host, "unknown")
	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal physedit-v1 status: %w", err)
	}
	return content, snapshotFromPhysEditStatus(status), nil
}

func (definition Default) ResolveThreadsProject(context Context, environ []string, explicit string) ThreadsProject {
	if strings.TrimSpace(explicit) != "" {
		return ThreadsProject{Name: explicit, Source: "flag"}
	}

	values := envMap(environ)
	if project := strings.TrimSpace(values["ANTON_THREADS_PROJECT"]); project != "" {
		return ThreadsProject{Name: project, Source: "env"}
	}

	if project := definition.projectFromWorkspaceRoot(context); project != "" {
		return ThreadsProject{Name: project, Source: "workspace-root"}
	}

	if definition.effectiveConfig().Threads.DefaultProjectStrategy == "" || definition.effectiveConfig().Threads.DefaultProjectStrategy == "repo-root" {
		if context.RepositoryRoot != "" {
			return ThreadsProject{
				Name:   filepath.Base(context.RepositoryRoot),
				Source: "repo-root",
			}
		}
	}

	return ThreadsProject{}
}

func (definition Default) resolvePath(context Context, pathValue string) string {
	base := context.WorkingDirectory
	if context.RepositoryRoot != "" {
		base = context.RepositoryRoot
	}
	if filepath.IsAbs(pathValue) {
		return pathValue
	}
	return filepath.Join(base, pathValue)
}

func (definition Default) tasksRoot(context Context) string {
	return definition.resolvePath(context, definition.effectiveConfig().Tasks.Root)
}

func (definition Default) taskLayout() string {
	return normalizedTaskLayout(definition.effectiveConfig().Tasks)
}

func (definition Default) statusSchema() string {
	return normalizedStatusSchema(definition.effectiveConfig().Tasks)
}

func (definition Default) projectFromWorkspaceRoot(context Context) string {
	for _, root := range definition.effectiveConfig().Threads.WorkspaceRoots {
		absoluteRoot := definition.resolvePath(context, root)
		prefix := absoluteRoot + string(filepath.Separator)
		if !strings.HasPrefix(context.WorkingDirectory, prefix) {
			continue
		}

		relative := strings.TrimPrefix(context.WorkingDirectory, prefix)
		parts := strings.Split(relative, string(filepath.Separator))
		if len(parts) == 0 {
			continue
		}
		project := trimString(parts[0])
		if project != "" {
			return ThreadsProject{
				Name:   project,
				Source: "workspace-root",
			}.Name
		}
	}
	return ""
}

func (definition Default) effectiveConfig() Config {
	if definition.Config.Version == 0 {
		return defaultConfig()
	}
	return definition.Config
}

func defaultTaskFiles(now time.Time) []TaskFile {
	return []TaskFile{
		{
			Name:     "task_plan.md",
			Template: "# Task Plan\n\n## Goal\n\n## Deliverables\n\n## Phases\n\n- [ ] Define the current task.\n",
		},
		{
			Name:     "findings.md",
			Template: "# Findings\n\n## Context\n\n## Observations\n",
		},
		{
			Name:     "progress.md",
			Template: "# Progress\n\n## " + now.UTC().Format("2006-01-02") + "\n\n- Initialized by `anton task-state init`.\n",
		},
	}
}

func (definition Default) topicLayerTaskBundle(context Context, environ []string, now time.Time, tasksRoot string) (ResolvedTaskBundle, error) {
	if current := currentTopicLayerTaskBundleRoot(context.WorkingDirectory, tasksRoot); current != "" {
		if err := ValidateTaskID(filepath.Base(current)); err != nil {
			return ResolvedTaskBundle{}, fmt.Errorf("current topic-layer task bundle root has invalid task id: %w", err)
		}
		return ResolvedTaskBundle{
			Root:          current,
			RequiredFiles: defaultTaskFiles(now),
			StatusFile:    "status.yaml",
		}, nil
	}

	taskID := inferTaskID(context, environ)
	if trimString(taskID) == "" {
		return ResolvedTaskBundle{}, TaskIdentityRequiredError{TasksRoot: tasksRoot}
	}
	if err := ValidateTaskID(taskID); err != nil {
		return ResolvedTaskBundle{}, fmt.Errorf("topic-layer task bundle root inferred invalid task id %q: %w", taskID, err)
	}
	if existing := findTopicLayerTaskBundle(tasksRoot, taskID); existing != "" {
		return ResolvedTaskBundle{
			Root:          existing,
			RequiredFiles: defaultTaskFiles(now),
			StatusFile:    "status.yaml",
		}, nil
	}

	values := envMap(environ)
	topic := strings.TrimSpace(values["ANTON_TASK_TOPIC"])
	if topic == "" {
		return ResolvedTaskBundle{}, fmt.Errorf("topic-layer task bundle %q not found under %s; set ANTON_TASK_TOPIC to create a new topic-layer bundle", taskID, filepath.ToSlash(tasksRoot))
	}
	if !safePathSegment(topic) {
		return ResolvedTaskBundle{}, fmt.Errorf("invalid ANTON_TASK_TOPIC %q: must be one path segment", topic)
	}

	return ResolvedTaskBundle{
		Root:          filepath.Join(tasksRoot, topic, "tasks", "active", taskID),
		RequiredFiles: defaultTaskFiles(now),
		StatusFile:    "status.yaml",
	}, nil
}

func currentAntonTaskBundleRoot(workingDirectory string, tasksRoot string) string {
	activePrefix := filepath.Join(tasksRoot, "active") + string(filepath.Separator)
	completedPrefix := filepath.Join(tasksRoot, "completed") + string(filepath.Separator)

	switch {
	case strings.HasPrefix(workingDirectory, activePrefix):
		return firstChildRoot(workingDirectory, activePrefix)
	case strings.HasPrefix(workingDirectory, completedPrefix):
		return firstChildRoot(workingDirectory, completedPrefix)
	default:
		return ""
	}
}

func currentTaskBundleRoot(workingDirectory string, tasksRoot string) string {
	return currentAntonTaskBundleRoot(workingDirectory, tasksRoot)
}

func currentTopicLayerTaskBundleRoot(workingDirectory string, tasksRoot string) string {
	relative, err := filepath.Rel(filepath.Clean(tasksRoot), filepath.Clean(workingDirectory))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for index := 0; index+2 < len(parts); index++ {
		if parts[index] != "tasks" || !taskLane(parts[index+1]) || trimString(parts[index+2]) == "" {
			continue
		}
		return filepath.Join(append([]string{tasksRoot}, parts[:index+3]...)...)
	}
	return ""
}

func findTopicLayerTaskBundle(tasksRoot string, taskID string) string {
	var found string
	_ = filepath.WalkDir(tasksRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if shouldSkipTopicLayerScanDir(entry.Name()) {
			return filepath.SkipDir
		}
		relative, relErr := filepath.Rel(filepath.Clean(tasksRoot), filepath.Clean(path))
		if relErr != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil
		}
		parts := strings.Split(relative, string(filepath.Separator))
		for index := 0; index+2 < len(parts); index++ {
			if parts[index] == "tasks" && taskLane(parts[index+1]) && parts[index+2] == taskID {
				found = filepath.Join(append([]string{tasksRoot}, parts[:index+3]...)...)
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

func shouldSkipTopicLayerScanDir(name string) bool {
	switch name {
	case ".git", ".worktrees", "archive", "archives", "__pycache__":
		return true
	default:
		return false
	}
}

func taskLane(value string) bool {
	switch value {
	case "active", "completed", "archived":
		return true
	default:
		return false
	}
}

func safePathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, `/\`)
}

func firstChildRoot(workingDirectory string, prefix string) string {
	relative := strings.TrimPrefix(workingDirectory, prefix)
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) == 0 || trimString(parts[0]) == "" {
		return ""
	}
	return filepath.Join(prefix[:len(prefix)-1], parts[0])
}

func inferTaskID(context Context, environ []string) string {
	values := envMap(environ)
	if taskID := strings.TrimSpace(values["ANTON_TASK_ID"]); taskID != "" {
		return taskID
	}

	if matches := genericTaskBranchPattern.FindStringSubmatch(context.GitBranch); len(matches) == 2 {
		return matches[1]
	}

	return ""
}

func validateDefaultStatus(path string, status defaultStatus) error {
	if status.Version != 1 {
		return fmt.Errorf("validate %s: unsupported status version %d", path, status.Version)
	}
	if trimString(status.Stable.TaskID) == "" {
		return fmt.Errorf("validate %s: missing stable.task_id", path)
	}
	if trimString(status.Stable.CreatedAt) == "" {
		return fmt.Errorf("validate %s: missing stable.created_at", path)
	}
	if trimString(status.State.Lifecycle) == "" {
		return fmt.Errorf("validate %s: missing state.lifecycle", path)
	}
	allowedLifecycle := map[string]bool{
		"active":  true,
		"blocked": true,
		"review":  true,
		"partial": true,
		"done":    true,
	}
	if !allowedLifecycle[trimString(status.State.Lifecycle)] {
		return fmt.Errorf("validate %s: unsupported state.lifecycle %q", path, status.State.Lifecycle)
	}
	if trimString(status.State.UpdatedAt) == "" {
		return fmt.Errorf("validate %s: missing state.updated_at", path)
	}
	if trimString(status.Machine.ExecutionTarget) == "" {
		return fmt.Errorf("validate %s: missing machine.execution_target", path)
	}
	if trimString(status.Machine.WorkingDirectory) == "" {
		return fmt.Errorf("validate %s: missing machine.working_directory", path)
	}
	if trimString(status.Machine.WorkspaceKind) == "" {
		return fmt.Errorf("validate %s: missing machine.workspace_kind", path)
	}
	if trimString(status.Closure.FinishState) == "" {
		return fmt.Errorf("validate %s: missing closure.finish_state", path)
	}
	if !allowedLifecycle[trimString(status.Closure.FinishState)] {
		return fmt.Errorf("validate %s: unsupported closure.finish_state %q", path, status.Closure.FinishState)
	}
	lifecycle := trimString(status.State.Lifecycle)
	if lifecycle == "blocked" || lifecycle == "review" || lifecycle == "partial" || lifecycle == "done" {
		if trimString(status.Closure.NextStep) == "" {
			return fmt.Errorf("validate %s: missing closure.next_step for lifecycle %s", path, lifecycle)
		}
	}
	return nil
}

func snapshotFromDefaultStatus(status defaultStatus) StatusSnapshot {
	return StatusSnapshot{
		TaskID:                   status.Stable.TaskID,
		Lifecycle:                status.State.Lifecycle,
		UpdatedAt:                status.State.UpdatedAt,
		FinishState:              status.Closure.FinishState,
		NextStep:                 status.Closure.NextStep,
		BlockerCount:             len(status.Closure.Blockers),
		ExpectedDeliverableCount: len(status.Closure.ExpectedDeliverables),
		AttemptCount:             len(status.Evidence.Attempts),
		ValidationCount:          len(status.Evidence.Validations),
	}
}

func readPhysEditStatus(path string) (StatusSnapshot, error) {
	status := physEditStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return StatusSnapshot{}, err
	}
	if err := validatePhysEditStatus(path, status); err != nil {
		return StatusSnapshot{}, err
	}
	return snapshotFromPhysEditStatus(status), nil
}

func validatePhysEditStatus(path string, status physEditStatus) error {
	if status.Version != 1 {
		return fmt.Errorf("validate %s: unsupported status version %d", path, status.Version)
	}
	if trimString(status.Task.ID) == "" {
		return fmt.Errorf("validate %s: missing task.id", path)
	}
	if trimString(status.Task.Topic) == "" {
		return fmt.Errorf("validate %s: missing task.topic", path)
	}
	if !physEditLifecycle(trimString(status.Task.Lifecycle)) {
		return fmt.Errorf("validate %s: unsupported task.lifecycle %q", path, status.Task.Lifecycle)
	}
	if trimString(status.Task.Phase) == "" {
		return fmt.Errorf("validate %s: missing task.phase", path)
	}
	if trimString(status.Task.Owner) == "" {
		return fmt.Errorf("validate %s: missing task.owner", path)
	}
	if trimString(status.Task.LastUpdated) == "" {
		return fmt.Errorf("validate %s: missing task.last_updated", path)
	}
	if trimString(status.Execution.CWD) == "" {
		return fmt.Errorf("validate %s: missing execution.cwd", path)
	}
	if trimString(status.Execution.Worktree) == "" && trimString(status.Execution.Checkout) == "" {
		return fmt.Errorf("validate %s: missing execution.worktree or execution.checkout", path)
	}
	if trimString(status.Environment.MachineType) == "" {
		return fmt.Errorf("validate %s: missing environment.machine_type", path)
	}
	if trimString(status.Environment.Proxy) == "" {
		return fmt.Errorf("validate %s: missing environment.proxy", path)
	}
	for index, service := range status.Services {
		if trimString(service.Name) == "" {
			return fmt.Errorf("validate %s: missing services[%d].name", path, index)
		}
		if trimString(service.Kind) == "" {
			return fmt.Errorf("validate %s: missing services[%d].kind", path, index)
		}
		if trimString(service.Status) == "" {
			return fmt.Errorf("validate %s: missing services[%d].status", path, index)
		}
		if trimString(service.Worktree) == "" {
			return fmt.Errorf("validate %s: missing services[%d].worktree", path, index)
		}
		if trimString(service.ReopenHint) == "" {
			return fmt.Errorf("validate %s: missing services[%d].reopen_hint", path, index)
		}
	}
	return nil
}

func snapshotFromPhysEditStatus(status physEditStatus) StatusSnapshot {
	nextStep := ""
	if len(status.State.NextAction) > 0 {
		nextStep = status.State.NextAction[0]
	}
	return StatusSnapshot{
		TaskID:                   status.Task.ID,
		Lifecycle:                status.Task.Lifecycle,
		UpdatedAt:                status.Task.LastUpdated,
		FinishState:              status.Task.Lifecycle,
		NextStep:                 nextStep,
		BlockerCount:             len(status.State.Blockers),
		ExpectedDeliverableCount: len(status.State.NextAction),
	}
}

func physEditLifecycle(value string) bool {
	switch value {
	case "active", "blocked", "completed", "archived":
		return true
	default:
		return false
	}
}

func topicFromBundleRoot(root string) string {
	clean := filepath.Clean(root)
	parts := strings.Split(clean, string(filepath.Separator))
	for index := 0; index+3 < len(parts); index++ {
		if parts[index+1] == "tasks" && taskLane(parts[index+2]) {
			return parts[index]
		}
	}
	return "unknown"
}

func fallbackString(value string, defaultValue string) string {
	if trimString(value) == "" {
		return defaultValue
	}
	return value
}
