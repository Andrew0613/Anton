package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"gopkg.in/yaml.v3"
)

type TaskRecord struct {
	TaskID          string           `json:"task_id" yaml:"task_id"`
	Topic           string           `json:"topic,omitempty" yaml:"topic"`
	Lane            string           `json:"lane,omitempty" yaml:"lane"`
	Lifecycle       string           `json:"lifecycle" yaml:"lifecycle"`
	TruthLocation   string           `json:"truth_location,omitempty" yaml:"truth_location"`
	Workspace       WorkspaceBinding `json:"workspace,omitempty" yaml:"workspace"`
	SourceRevision  string           `json:"source_revision,omitempty" yaml:"source_revision"`
	Freshness       Freshness        `json:"freshness,omitempty" yaml:"freshness"`
	Blockers        []string         `json:"blockers,omitempty" yaml:"blockers"`
	CloseoutAllowed bool             `json:"closeout_allowed,omitempty" yaml:"closeout_allowed"`
	Active          bool             `json:"active,omitempty" yaml:"active"`
	SourceFile      string           `json:"source_file"`
}

type WorkspaceBinding struct {
	Path   string `json:"path,omitempty" yaml:"path"`
	Branch string `json:"branch,omitempty" yaml:"branch"`
	Head   string `json:"head,omitempty" yaml:"head"`
}

type Freshness struct {
	Status    string `json:"status,omitempty" yaml:"status"`
	UpdatedAt string `json:"updated_at,omitempty" yaml:"updated_at"`
}

type Issue struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	RuleID     string `json:"rule_id,omitempty"`
	File       string `json:"file,omitempty"`
	Message    string `json:"message"`
	RepairHint string `json:"repair_hint,omitempty"`
}

type LegacyTaskRecord struct {
	TaskID         string `json:"task_id"`
	Topic          string `json:"topic,omitempty"`
	Lifecycle      string `json:"lifecycle,omitempty"`
	Phase          string `json:"phase,omitempty"`
	SourceFile     string `json:"source_file"`
	Classification string `json:"classification"`
	Reason         string `json:"reason,omitempty"`
}

type Inventory struct {
	StateRoot          string             `json:"state_root"`
	TasksDir           string             `json:"tasks_dir"`
	SourceRevision     string             `json:"source_revision,omitempty"`
	Tasks              []TaskRecord       `json:"tasks"`
	Active             []TaskRecord       `json:"active"`
	LegacyTasks        []LegacyTaskRecord `json:"legacy_tasks,omitempty"`
	LegacyActive       []LegacyTaskRecord `json:"legacy_active,omitempty"`
	Issues             []Issue            `json:"issues,omitempty"`
	DualReadEnabled    bool               `json:"dual_read_enabled"`
	LegacyTasksRoot    string             `json:"legacy_tasks_root"`
	LegacyStatusSchema string             `json:"legacy_status_schema"`
}

func LoadInventory(resolved adapter.Resolved, dualRead bool) (Inventory, error) {
	base := resolved.Context.WorkingDirectory
	if resolved.Context.RepositoryRoot != "" {
		base = resolved.Context.RepositoryRoot
	}
	stateRoot := filepath.Clean(filepath.Join(base, resolved.Config.StateRoot()))
	tasksDir := filepath.Join(stateRoot, "tasks")
	inventory := Inventory{
		StateRoot:          stateRoot,
		TasksDir:           tasksDir,
		Tasks:              []TaskRecord{},
		Active:             []TaskRecord{},
		Issues:             []Issue{},
		DualReadEnabled:    dualRead,
		LegacyTasksRoot:    filepath.Clean(filepath.Join(base, resolved.Config.Tasks.Root)),
		LegacyStatusSchema: resolved.Config.Tasks.StatusSchema,
	}

	revision := readRepositoryHead(base)
	if revision != "" {
		inventory.SourceRevision = revision
	}

	tasksDirExists := false
	if info, err := os.Stat(tasksDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			inventory.Issues = append(inventory.Issues, Issue{
				Level:      "warning",
				Code:       "state-tasks-dir-missing",
				RuleID:     "state.inventory.root_exists",
				File:       tasksDir,
				Message:    "state tasks directory does not exist",
				RepairHint: "create docs/state/tasks and add task projection files",
			})
		} else {
			return inventory, fmt.Errorf("stat %s: %w", tasksDir, err)
		}
	} else if !info.IsDir() {
		return inventory, fmt.Errorf("state tasks path is not a directory: %s", tasksDir)
	} else {
		tasksDirExists = true
	}

	if tasksDirExists {
		if err := filepath.WalkDir(tasksDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			record, parseErr := parseTaskFile(tasksDir, path)
			if parseErr != nil {
				inventory.Issues = append(inventory.Issues, Issue{
					Level:      "error",
					Code:       "state-task-parse-failed",
					RuleID:     "state.inventory.file_schema",
					File:       path,
					Message:    parseErr.Error(),
					RepairHint: "fix task projection YAML fields or move malformed file into archive",
				})
				return nil
			}
			inventory.Tasks = append(inventory.Tasks, record)
			if isActive(record) {
				inventory.Active = append(inventory.Active, record)
			}
			return nil
		}); err != nil {
			return inventory, err
		}
	}

	if dualRead {
		legacyInventory, issues, err := scanLegacyInventory(inventory.LegacyTasksRoot, resolved.Config)
		if err != nil {
			return inventory, err
		}
		inventory.LegacyTasks = legacyInventory.Tasks
		inventory.LegacyActive = legacyInventory.Active
		inventory.Issues = append(inventory.Issues, issues...)
		inventory.Issues = append(inventory.Issues, checkDualReadParity(inventory.LegacyTasksRoot, resolved.Config, inventory.Active, legacyInventory)...)
	}
	return inventory, nil
}

func ResolveTask(resolved adapter.Resolved, taskID string, dualRead bool) (TaskRecord, []Issue, error) {
	inventory, err := LoadInventory(resolved, dualRead)
	if err != nil {
		return TaskRecord{}, nil, err
	}
	issues := append([]Issue{}, inventory.Issues...)
	if strings.TrimSpace(taskID) != "" {
		for _, task := range inventory.Tasks {
			if task.TaskID == strings.TrimSpace(taskID) {
				return task, issues, nil
			}
		}
		issues = append(issues, Issue{
			Level:      "error",
			Code:       "state-task-not-found",
			RuleID:     "state.resolve.exists",
			Message:    fmt.Sprintf("task %q not found in %s", strings.TrimSpace(taskID), inventory.TasksDir),
			RepairHint: "check task_id spelling or refresh docs/state/tasks projections",
		})
		return TaskRecord{}, issues, nil
	}

	if len(inventory.Active) == 1 {
		return inventory.Active[0], issues, nil
	}
	if len(inventory.Active) == 0 {
		issues = append(issues, Issue{
			Level:      "error",
			Code:       "state-active-empty",
			RuleID:     "state.resolve.active_singleton",
			Message:    "no active task found in state inventory",
			RepairHint: "mark one task as active:true or lifecycle: active in docs/state/tasks",
		})
		return TaskRecord{}, issues, nil
	}
	ids := make([]string, 0, len(inventory.Active))
	for _, item := range inventory.Active {
		ids = append(ids, item.TaskID)
	}
	issues = append(issues, Issue{
		Level:      "error",
		Code:       "state-active-conflict",
		RuleID:     "state.resolve.active_singleton",
		Message:    fmt.Sprintf("multiple active tasks found: %s", strings.Join(ids, ", ")),
		RepairHint: "leave exactly one task active before using implicit resolution",
	})
	return TaskRecord{}, issues, nil
}

func parseTaskFile(tasksDir string, path string) (TaskRecord, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return TaskRecord{}, err
	}
	record := TaskRecord{}
	if err := yaml.Unmarshal(content, &record); err != nil {
		return TaskRecord{}, err
	}
	record.SourceFile = filepath.ToSlash(relativeOrAbsolute(tasksDir, path))
	record.TaskID = strings.TrimSpace(record.TaskID)
	record.Lifecycle = strings.TrimSpace(record.Lifecycle)
	if record.TaskID == "" {
		return TaskRecord{}, errors.New("task_id is required")
	}
	if record.Lifecycle == "" {
		return TaskRecord{}, errors.New("lifecycle is required")
	}
	if err := adapter.ValidateTaskID(record.TaskID); err != nil {
		return TaskRecord{}, err
	}
	return record, nil
}

func isActive(record TaskRecord) bool {
	if record.Active {
		return true
	}
	lifecycle := strings.ToLower(strings.TrimSpace(record.Lifecycle))
	return lifecycle == "active"
}

func checkDualReadParity(legacyRoot string, config adapter.Config, active []TaskRecord, legacy legacyInventory) []Issue {
	issues := []Issue{}
	if usesTopicLayerLegacy(config) {
		activeByID := map[string]TaskRecord{}
		for _, item := range active {
			activeByID[item.TaskID] = item
		}
		legacyActiveByID := map[string]LegacyTaskRecord{}
		for _, item := range legacy.Active {
			legacyActiveByID[item.TaskID] = item
			if _, found := activeByID[item.TaskID]; found {
				continue
			}
			issues = append(issues, Issue{
				Level:      "error",
				Code:       "state-dual-read-missing-state-projection",
				RuleID:     "state.dual_read.state_projection",
				File:       filepath.Join(legacyRoot, item.SourceFile),
				Message:    fmt.Sprintf("current legacy active task %q has no active docs/state task projection", item.TaskID),
				RepairHint: "create or refresh docs/state/tasks projection with lifecycle: active for this task",
			})
		}
		for _, item := range active {
			if _, found := legacyActiveByID[item.TaskID]; found {
				continue
			}
			issues = append(issues, Issue{
				Level:      "error",
				Code:       "state-dual-read-missing-current-legacy",
				RuleID:     "state.dual_read.current_legacy_presence",
				File:       item.SourceFile,
				Message:    fmt.Sprintf("active state task %q has no matching current legacy active status.yaml", item.TaskID),
				RepairHint: "create/update project_progress/<Topic>/tasks/active/<id>/status.yaml with task.lifecycle: active or move the state projection out of active",
			})
		}
		return issues
	}

	for _, item := range active {
		found := hasLegacyStatus(legacyRoot, config, item.TaskID)
		if found {
			continue
		}
		issues = append(issues, Issue{
			Level:      "error",
			Code:       "state-dual-read-missing-current-legacy",
			RuleID:     "state.dual_read.current_legacy_presence",
			File:       legacyRoot,
			Message:    fmt.Sprintf("active state task %q has no matching current legacy status.yaml", item.TaskID),
			RepairHint: "either create/update legacy projection during parity window or classify the task as archive-only",
		})
	}
	return issues
}

func hasLegacyStatus(legacyRoot string, config adapter.Config, taskID string) bool {
	primary := filepath.Join(legacyRoot, "active", taskID, "status.yaml")
	if statFile(primary) {
		return true
	}
	if strings.EqualFold(config.Tasks.Layout, "topic-layer") || config.Tasks.TopicLayer {
		entries, err := os.ReadDir(legacyRoot)
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(legacyRoot, entry.Name(), "tasks", "active", taskID, "status.yaml")
			if statFile(candidate) {
				return true
			}
		}
	}
	return false
}

func statFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func relativeOrAbsolute(base string, target string) string {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return relative
}

func readRepositoryHead(repoRoot string) string {
	headPath := filepath.Join(repoRoot, ".git", "HEAD")
	if !statFile(headPath) {
		gitFile := filepath.Join(repoRoot, ".git")
		content, err := os.ReadFile(gitFile)
		if err == nil {
			line := strings.TrimSpace(string(content))
			if strings.HasPrefix(line, "gitdir: ") {
				gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Clean(filepath.Join(repoRoot, gitDir))
				}
				headPath = filepath.Join(gitDir, "HEAD")
			}
		}
	}
	content, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}
