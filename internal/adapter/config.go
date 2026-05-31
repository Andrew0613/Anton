package adapter

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Version      int                          `yaml:"version"`
	Entrypoint   EntrypointConfig             `yaml:"entrypoint"`
	Tasks        TasksConfig                  `yaml:"tasks"`
	Run          RunConfig                    `yaml:"run"`
	Threads      ThreadsConfig                `yaml:"threads"`
	Migrate      MigrateConfig                `yaml:"migrate"`
	Gates        []GateConfig                 `yaml:"gates"`
	GateProfiles map[string]GateProfileConfig `yaml:"gate_profiles"`
	Roots        RootsConfig                  `yaml:"roots"`
	Extensions   ExtensionsConfig             `yaml:"extensions"`
	Path         string                       `yaml:"-"`
	Loaded       bool                         `yaml:"-"`
	Inherited    bool                         `yaml:"-"`
}

type EntrypointConfig struct {
	Path string `yaml:"path"`
}

type TasksConfig struct {
	Root         string `yaml:"root"`
	Layout       string `yaml:"layout"`
	TopicLayer   bool   `yaml:"topic_layer"`
	StatusSchema string `yaml:"status_schema"`
	CardSync     bool   `yaml:"card_sync"`
	PlanningMode string `yaml:"planning_mode"`
}

type RunConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Manifest    string `yaml:"manifest"`
	ReceiptsDir string `yaml:"receipts_dir"`
}

type ThreadsConfig struct {
	DefaultProjectStrategy string   `yaml:"default_project_strategy"`
	WorkspaceRoots         []string `yaml:"workspace_roots"`
}

type MigrateConfig struct {
	TargetSchema  MigrateTargetSchemaConfig `yaml:"target_schema"`
	DefaultTarget string                    `yaml:"default_target"`
}

type MigrateTargetSchemaConfig struct {
	Version int    `yaml:"version"`
	Locked  bool   `yaml:"locked"`
	Reason  string `yaml:"reason"`
}

type RootsConfig struct {
	StateRoot          string `yaml:"state"`
	MemoryRoot         string `yaml:"memory"`
	ArtifactRoot       string `yaml:"artifacts"`
	ArchiveRoot        string `yaml:"archive"`
	ViewRoot           string `yaml:"views"`
	PolicyRegistryRoot string `yaml:"policy_registry"`
}

type GateConfig struct {
	Name        string             `yaml:"name"`
	Type        string             `yaml:"type"`
	RequiredFor []string           `yaml:"required_for"`
	Description string             `yaml:"description"`
	Command     *GateCommandConfig `yaml:"command"`
	Timeout     *GateTimeoutConfig `yaml:"timeout"`
	Destructive bool               `yaml:"destructive"`
}

type GateCommandConfig struct {
	Argv             []string `yaml:"argv"`
	WorkingDirectory string   `yaml:"working_directory"`
}

type GateTimeoutConfig struct {
	Seconds int `yaml:"seconds"`
}

type GateProfileConfig struct {
	Required []string `yaml:"required"`
}

type ExtensionsConfig struct {
	History HistoryExtensionConfig `yaml:"history"`
}

type HistoryExtensionConfig struct {
	WorkRecordRoots []string `yaml:"work_record_roots"`
}

func LoadConfig(context Context) (Config, error) {
	config := defaultConfig()
	validationPath := ""

	base := context.WorkingDirectory
	if context.RepositoryRoot != "" {
		base = context.RepositoryRoot
	}

	configPath := filepath.Join(base, "anton.yaml")
	validationPath = configPath
	if _, err := os.Stat(configPath); err == nil {
		if err := readYAMLFileStrict(configPath, &config); err != nil {
			return Config{}, wrapConfigError(configPath, err)
		}
		config.Path = configPath
		config.Loaded = true
	} else if os.IsNotExist(err) {
		if inheritedPath := inheritedWorktreeConfigPath(context); inheritedPath != "" {
			if err := readYAMLFileStrict(inheritedPath, &config); err != nil {
				return Config{}, wrapConfigError(inheritedPath, err)
			}
			config.Path = inheritedPath
			validationPath = inheritedPath
			config.Loaded = true
			config.Inherited = true
		} else {
			config.Path = configPath
		}
	} else if err != nil {
		return Config{}, fmt.Errorf("stat %s: %w", configPath, err)
	}

	if err := validateConfig(config); err != nil {
		return Config{}, wrapConfigError(validationPath, err)
	}
	return config, nil
}

func inheritedWorktreeConfigPath(context Context) string {
	if context.RepositoryKind != "git-worktree" || context.RepositoryRoot == "" {
		return ""
	}

	gitDir, err := resolveGitDir(context.RepositoryRoot)
	if err != nil {
		return ""
	}
	commonDir := gitDir
	commonPath := filepath.Join(gitDir, "commondir")
	if content, err := os.ReadFile(commonPath); err == nil {
		value := trimString(string(content))
		if value != "" {
			if filepath.IsAbs(value) {
				commonDir = filepath.Clean(value)
			} else {
				commonDir = filepath.Clean(filepath.Join(gitDir, value))
			}
		}
	}

	if filepath.Base(commonDir) != ".git" {
		return ""
	}
	mainCheckout := filepath.Dir(commonDir)
	if filepath.Clean(mainCheckout) == filepath.Clean(context.RepositoryRoot) {
		return ""
	}

	configPath := filepath.Join(mainCheckout, "anton.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}
	return ""
}

func defaultConfig() Config {
	return Config{
		Version: 1,
		Entrypoint: EntrypointConfig{
			Path: "AGENTS.md",
		},
		Tasks: TasksConfig{
			Root:         ".anton/tasks",
			PlanningMode: "planning_files",
		},
		Run: RunConfig{
			Manifest:    "run.json",
			ReceiptsDir: "receipts",
		},
		Threads: ThreadsConfig{
			DefaultProjectStrategy: "repo-root",
			WorkspaceRoots:         []string{},
		},
		Roots: RootsConfig{
			StateRoot:          "docs/state",
			MemoryRoot:         "docs/memory",
			ArtifactRoot:       "docs/artifacts",
			ArchiveRoot:        "docs/archive",
			ViewRoot:           "docs/views",
			PolicyRegistryRoot: "docs/agent-workflow/registries",
		},
		Gates:        []GateConfig{},
		GateProfiles: map[string]GateProfileConfig{},
	}
}

func validateConfig(config Config) error {
	if config.Version != 1 {
		return fmt.Errorf("unsupported anton config version %d", config.Version)
	}
	if trimString(config.Entrypoint.Path) == "" {
		return fmt.Errorf("anton config entrypoint.path must not be empty")
	}
	if trimString(config.Tasks.Root) == "" {
		return fmt.Errorf("anton config tasks.root must not be empty")
	}
	switch normalizedTaskLayout(config.Tasks) {
	case "anton", "topic-layer":
	default:
		return fmt.Errorf("anton config tasks.layout must be one of: anton, topic-layer")
	}
	switch normalizedStatusSchema(config.Tasks) {
	case "anton", "physedit-v1":
	default:
		return fmt.Errorf("anton config tasks.status_schema must be one of: anton, physedit-v1")
	}
	switch normalizedPlanningMode(config.Tasks) {
	case "planning_files", "run_manifest", "hybrid":
	default:
		return fmt.Errorf("anton config tasks.planning_mode must be one of: planning_files, run_manifest, hybrid")
	}
	if trimString(config.Run.Manifest) == "" {
		return fmt.Errorf("anton config run.manifest must not be empty")
	}
	if trimString(config.Run.ReceiptsDir) == "" {
		return fmt.Errorf("anton config run.receipts_dir must not be empty")
	}
	switch config.Threads.DefaultProjectStrategy {
	case "repo-root", "none":
	default:
		return fmt.Errorf("anton config threads.default_project_strategy must be one of: repo-root, none")
	}
	for index, root := range config.Threads.WorkspaceRoots {
		if trimString(root) == "" {
			return fmt.Errorf("anton config threads.workspace_roots[%d] must not be empty", index)
		}
	}
	if config.Migrate.TargetSchema.Version != 0 && config.Migrate.TargetSchema.Version != 2 {
		return fmt.Errorf("anton config migrate.target_schema.version must be 2")
	}
	if config.Migrate.TargetSchema.Locked && config.Migrate.TargetSchema.Version != 2 {
		return fmt.Errorf("anton config migrate.target_schema.version must be 2 when locked")
	}
	if config.Migrate.DefaultTarget != "" && trimString(config.Migrate.DefaultTarget) == "" {
		return fmt.Errorf("anton config migrate.default_target must not be empty when declared")
	}
	if config.Roots.StateRoot != "" && trimString(config.Roots.StateRoot) == "" {
		return fmt.Errorf("anton config roots.state must not be empty when declared")
	}
	if config.Roots.MemoryRoot != "" && trimString(config.Roots.MemoryRoot) == "" {
		return fmt.Errorf("anton config roots.memory must not be empty when declared")
	}
	if config.Roots.ArtifactRoot != "" && trimString(config.Roots.ArtifactRoot) == "" {
		return fmt.Errorf("anton config roots.artifacts must not be empty when declared")
	}
	if config.Roots.ArchiveRoot != "" && trimString(config.Roots.ArchiveRoot) == "" {
		return fmt.Errorf("anton config roots.archive must not be empty when declared")
	}
	if config.Roots.ViewRoot != "" && trimString(config.Roots.ViewRoot) == "" {
		return fmt.Errorf("anton config roots.views must not be empty when declared")
	}
	if config.Roots.PolicyRegistryRoot != "" && trimString(config.Roots.PolicyRegistryRoot) == "" {
		return fmt.Errorf("anton config roots.policy_registry must not be empty when declared")
	}
	for index, root := range config.Extensions.History.WorkRecordRoots {
		if trimString(root) == "" {
			return fmt.Errorf("anton config extensions.history.work_record_roots[%d] must not be empty", index)
		}
	}
	for name, profile := range config.GateProfiles {
		if trimString(name) == "" {
			return fmt.Errorf("anton config gate_profiles contains an empty profile name")
		}
		for index, gate := range profile.Required {
			if trimString(gate) == "" {
				return fmt.Errorf("anton config gate_profiles.%s.required[%d] must not be empty", name, index)
			}
		}
	}
	return nil
}

func normalizedTaskLayout(tasks TasksConfig) string {
	if tasks.TopicLayer {
		return "topic-layer"
	}
	layout := trimString(tasks.Layout)
	if layout == "" {
		return "anton"
	}
	return layout
}

func normalizedStatusSchema(tasks TasksConfig) string {
	schema := trimString(tasks.StatusSchema)
	if schema == "" {
		return "anton"
	}
	return schema
}

func normalizedPlanningMode(tasks TasksConfig) string {
	mode := trimString(tasks.PlanningMode)
	if mode == "" {
		return "planning_files"
	}
	return mode
}

func (config Config) PlanningMode() string {
	return normalizedPlanningMode(config.Tasks)
}

func (config Config) MigrateTargetSchemaVersion() int {
	if config.Migrate.TargetSchema.Version == 0 {
		return 2
	}
	return config.Migrate.TargetSchema.Version
}

func (config Config) MigrateTargetSchemaLocked() bool {
	return config.Migrate.TargetSchema.Locked
}

func (config Config) MigrateTargetSchemaReason() string {
	reason := trimString(config.Migrate.TargetSchema.Reason)
	if reason != "" {
		return reason
	}
	if config.MigrateTargetSchemaLocked() {
		return "v2 config schema is locked by anton.yaml"
	}
	return "v2 config schema is not locked"
}

func (config Config) MigrateDefaultTarget() string {
	value := trimString(config.Migrate.DefaultTarget)
	if value != "" {
		return value
	}
	return trimString(config.Tasks.Root)
}

func (config Config) RunManifestName() string {
	value := trimString(config.Run.Manifest)
	if value == "" {
		return "run.json"
	}
	return value
}

func (config Config) RunReceiptsDir() string {
	value := trimString(config.Run.ReceiptsDir)
	if value == "" {
		return "receipts"
	}
	return value
}

func (config Config) StateRoot() string {
	value := trimString(config.Roots.StateRoot)
	if value == "" {
		return "docs/state"
	}
	return value
}

func (config Config) MemoryRoot() string {
	value := trimString(config.Roots.MemoryRoot)
	if value == "" {
		return "docs/memory"
	}
	return value
}

func (config Config) ArtifactRoot() string {
	value := trimString(config.Roots.ArtifactRoot)
	if value == "" {
		return "docs/artifacts"
	}
	return value
}

func (config Config) ArchiveRoot() string {
	value := trimString(config.Roots.ArchiveRoot)
	if value == "" {
		return "docs/archive"
	}
	return value
}

func (config Config) ViewRoot() string {
	value := trimString(config.Roots.ViewRoot)
	if value == "" {
		return "docs/views"
	}
	return value
}

func (config Config) PolicyRegistryRoot() string {
	value := trimString(config.Roots.PolicyRegistryRoot)
	if value == "" {
		return "docs/agent-workflow/registries"
	}
	return value
}

func wrapConfigError(path string, err error) error {
	return fmt.Errorf("invalid anton config at %s: %w", path, err)
}

func (config Config) Source() string {
	if config.Loaded && config.Inherited {
		return "inherited main-checkout anton.yaml"
	}
	if config.Loaded {
		return "repo-local anton.yaml"
	}
	return "built-in defaults"
}
