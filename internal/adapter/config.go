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
	Gates        []GateConfig                 `yaml:"gates"`
	GateProfiles map[string]GateProfileConfig `yaml:"gate_profiles"`
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
