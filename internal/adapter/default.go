package adapter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var genericTaskBranchPattern = regexp.MustCompile(`^task(?:/[^/]+)*/([^/]+)$`)

type Default struct {
	Config Config
}

func (Default) Name() string {
	return "canonical"
}

func (definition Default) TaskBundle(context Context, environ []string, now time.Time) (ResolvedTaskBundle, error) {
	tasksRoot := definition.tasksRoot(context)
	if current := currentTaskBundleRoot(context.WorkingDirectory, tasksRoot); current != "" {
		return ResolvedTaskBundle{
			Root: current,
			RequiredFiles: []TaskFile{
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
			},
			StatusFile: "status.yaml",
		}, nil
	}

	taskID := inferTaskID(context, environ)
	if trimString(taskID) == "" {
		return ResolvedTaskBundle{}, fmt.Errorf("canonical task bundle root could not be inferred; set ANTON_TASK_ID, use a task/<id_slug> branch, or run inside an existing %s bundle", filepath.ToSlash(tasksRoot))
	}

	return ResolvedTaskBundle{
		Root: filepath.Join(tasksRoot, "active", taskID),
		RequiredFiles: []TaskFile{
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
		},
		StatusFile: "status.yaml",
	}, nil
}

func (definition Default) EntrypointPath(context Context) string {
	return definition.resolvePath(context, definition.effectiveConfig().Entrypoint.Path)
}

type defaultStatus struct {
	Version int                  `yaml:"version"`
	Stable  defaultStableSection `yaml:"stable"`
	State   defaultStateSection  `yaml:"state"`
	Machine defaultMachine       `yaml:"machine"`
	Extra   map[string]any       `yaml:",inline"`
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

type defaultMachine struct {
	Host             string         `yaml:"host"`
	ExecutionTarget  string         `yaml:"execution_target"`
	WorkingDirectory string         `yaml:"working_directory"`
	WorkspaceKind    string         `yaml:"workspace_kind"`
	Extra            map[string]any `yaml:",inline"`
}

func (Default) ReadStatus(path string) (StatusSnapshot, error) {
	status := defaultStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return StatusSnapshot{}, err
	}

	if err := validateDefaultStatus(path, status); err != nil {
		return StatusSnapshot{}, err
	}

	return StatusSnapshot{TaskID: status.Stable.TaskID}, nil
}

func (Default) InitStatus(context Context, bundle ResolvedTaskBundle, now time.Time) ([]byte, StatusSnapshot, error) {
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
	}

	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal default status: %w", err)
	}
	return content, StatusSnapshot{TaskID: taskID}, nil
}

func (Default) PulseStatus(path string, context Context, now time.Time) ([]byte, StatusSnapshot, error) {
	status := defaultStatus{}
	if err := readYAMLFile(path, &status); err != nil {
		return nil, StatusSnapshot{}, err
	}
	if err := validateDefaultStatus(path, status); err != nil {
		return nil, StatusSnapshot{}, err
	}

	status.State.Lifecycle = "active"
	status.State.UpdatedAt = now.UTC().Format(time.RFC3339)
	status.Machine.Host = fallbackString(context.Host, "unknown")
	status.Machine.ExecutionTarget = context.ExecutionTarget
	status.Machine.WorkingDirectory = context.WorkingDirectory
	status.Machine.WorkspaceKind = context.WorkspaceKind

	content, err := marshalYAML(status)
	if err != nil {
		return nil, StatusSnapshot{}, fmt.Errorf("marshal default status: %w", err)
	}
	return content, StatusSnapshot{TaskID: status.Stable.TaskID}, nil
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

func currentTaskBundleRoot(workingDirectory string, tasksRoot string) string {
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
	return nil
}

func fallbackString(value string, defaultValue string) string {
	if trimString(value) == "" {
		return defaultValue
	}
	return value
}
