package contract

import (
	"runtime"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const SchemaVersion = "anton.contract.v1"

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

type Finding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

type Environment struct {
	ExecutionTarget string `json:"execution_target"`
	Host            string `json:"host"`
	OperatingSystem string `json:"operating_system"`
	Architecture    string `json:"architecture"`
	FilesystemType  string `json:"filesystem_type,omitempty"`
}

type Context struct {
	WorkingDirectory string   `json:"working_directory"`
	WorkspaceKind    string   `json:"workspace_kind"`
	RepositoryRoot   string   `json:"repository_root,omitempty"`
	RepositoryKind   string   `json:"repository_kind,omitempty"`
	GitBranch        string   `json:"git_branch,omitempty"`
	ScopePaths       []string `json:"scope_paths"`
}

type Config struct {
	Path                          string   `json:"path"`
	Source                        string   `json:"source"`
	EntrypointPath                string   `json:"entrypoint_path"`
	TasksRoot                     string   `json:"tasks_root"`
	TasksPlanningMode             string   `json:"tasks_planning_mode"`
	RunEnabled                    bool     `json:"run_enabled"`
	RunManifest                   string   `json:"run_manifest"`
	RunReceiptsDir                string   `json:"run_receipts_dir"`
	ThreadsDefaultProjectStrategy string   `json:"threads_default_project_strategy"`
	ThreadsWorkspaceRoots         []string `json:"threads_workspace_roots,omitempty"`
	StateRoot                     string   `json:"state_root"`
	MemoryRoot                    string   `json:"memory_root"`
	ArtifactRoot                  string   `json:"artifact_root"`
	ArchiveRoot                   string   `json:"archive_root"`
	ViewRoot                      string   `json:"view_root"`
	PolicyRegistryRoot            string   `json:"policy_registry_root"`
}

type Summary struct {
	Status        string `json:"status"`
	OKCount       int    `json:"ok_count"`
	DegradedCount int    `json:"degraded_count"`
	BlockedCount  int    `json:"blocked_count"`
}

type ContractV1 struct {
	SchemaVersion  string               `json:"schema_version"`
	Adapter        string               `json:"adapter"`
	Environment    Environment          `json:"environment"`
	Context        Context              `json:"context"`
	Config         Config               `json:"config"`
	TaskIdentity   adapter.TaskIdentity `json:"task_identity"`
	Checks         []Check              `json:"checks"`
	Findings       []Finding            `json:"findings,omitempty"`
	Summary        Summary              `json:"summary"`
	PromptContract string               `json:"prompt_contract"`
}

type Input struct {
	Adapter        string
	Context        adapter.Context
	Config         adapter.Config
	EntrypointPath string
	TaskIdentity   adapter.TaskIdentity
	FilesystemType string
	Checks         []Check
	Findings       []Finding
	Summary        Summary
}

func Build(input Input) ContractV1 {
	context := Context{
		WorkingDirectory: input.Context.WorkingDirectory,
		WorkspaceKind:    input.Context.WorkspaceKind,
		RepositoryRoot:   input.Context.RepositoryRoot,
		RepositoryKind:   input.Context.RepositoryKind,
		GitBranch:        input.Context.GitBranch,
		ScopePaths:       input.Context.ScopePaths,
	}

	return ContractV1{
		SchemaVersion: SchemaVersion,
		Adapter:       input.Adapter,
		Environment: Environment{
			ExecutionTarget: input.Context.ExecutionTarget,
			Host:            input.Context.Host,
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
			FilesystemType:  input.FilesystemType,
		},
		Context: context,
		Config: Config{
			Path:                          input.Config.Path,
			Source:                        input.Config.Source(),
			EntrypointPath:                input.EntrypointPath,
			TasksRoot:                     input.Config.Tasks.Root,
			TasksPlanningMode:             input.Config.PlanningMode(),
			RunEnabled:                    input.Config.Run.Enabled,
			RunManifest:                   input.Config.RunManifestName(),
			RunReceiptsDir:                input.Config.RunReceiptsDir(),
			ThreadsDefaultProjectStrategy: input.Config.Threads.DefaultProjectStrategy,
			ThreadsWorkspaceRoots:         input.Config.Threads.WorkspaceRoots,
			StateRoot:                     input.Config.StateRoot(),
			MemoryRoot:                    input.Config.MemoryRoot(),
			ArtifactRoot:                  input.Config.ArtifactRoot(),
			ArchiveRoot:                   input.Config.ArchiveRoot(),
			ViewRoot:                      input.Config.ViewRoot(),
			PolicyRegistryRoot:            input.Config.PolicyRegistryRoot(),
		},
		TaskIdentity:   input.TaskIdentity,
		Checks:         input.Checks,
		Findings:       input.Findings,
		Summary:        input.Summary,
		PromptContract: RenderPromptContract(input.Context.ExecutionTarget, context, input.TaskIdentity),
	}
}

func RenderPromptContract(executionTarget string, context Context, identity adapter.TaskIdentity) string {
	lines := []string{
		"Execution target: " + executionTarget,
		"Working directory: " + context.WorkingDirectory,
		"Workspace kind: " + context.WorkspaceKind,
	}

	if context.RepositoryRoot != "" {
		lines = append(lines, "Repository root: "+context.RepositoryRoot)
	}
	if context.RepositoryKind != "" {
		lines = append(lines, "Repository kind: "+context.RepositoryKind)
	}
	if context.GitBranch != "" {
		lines = append(lines, "Git branch: "+context.GitBranch)
	}
	if len(context.ScopePaths) > 0 {
		lines = append(lines, "Scope paths: "+strings.Join(context.ScopePaths, ", "))
	}
	if identity.Conflict {
		lines = append(lines, "Task identity conflict: "+strings.Join(identity.ConflictValues, ", "))
	} else if strings.TrimSpace(identity.Resolved) != "" {
		lines = append(lines, "Inferred task id: "+identity.Resolved)
	} else {
		lines = append(lines, "Inferred task id: unresolved")
	}

	return strings.Join(lines, "\n")
}
