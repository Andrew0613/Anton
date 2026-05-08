package adapter

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Version    int              `yaml:"version"`
	Entrypoint EntrypointConfig `yaml:"entrypoint"`
	Tasks      TasksConfig      `yaml:"tasks"`
	Threads    ThreadsConfig    `yaml:"threads"`
	Path       string           `yaml:"-"`
	Loaded     bool             `yaml:"-"`
	Inherited  bool             `yaml:"-"`
}

type EntrypointConfig struct {
	Path string `yaml:"path"`
}

type TasksConfig struct {
	Root string `yaml:"root"`
}

type ThreadsConfig struct {
	DefaultProjectStrategy string   `yaml:"default_project_strategy"`
	WorkspaceRoots         []string `yaml:"workspace_roots"`
}

func LoadConfig(context Context) (Config, error) {
	config := defaultConfig()

	base := context.WorkingDirectory
	if context.RepositoryRoot != "" {
		base = context.RepositoryRoot
	}

	configPath := filepath.Join(base, "anton.yaml")
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
			config.Loaded = true
			config.Inherited = true
		} else {
			config.Path = configPath
		}
	} else if err != nil {
		return Config{}, fmt.Errorf("stat %s: %w", configPath, err)
	}

	if err := validateConfig(config); err != nil {
		return Config{}, wrapConfigError(configPath, err)
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
			Root: ".anton/tasks",
		},
		Threads: ThreadsConfig{
			DefaultProjectStrategy: "repo-root",
			WorkspaceRoots:         []string{},
		},
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
	return nil
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
