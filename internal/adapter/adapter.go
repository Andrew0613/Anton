package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Context struct {
	WorkingDirectory string
	WorkspaceKind    string
	RepositoryRoot   string
	RepositoryKind   string
	GitBranch        string
	ScopePaths       []string
	ExecutionTarget  string
	Host             string
}

type TaskFile struct {
	Name     string
	Template string
}

type TaskBundle struct {
	RequiredFiles []TaskFile
	StatusFile    string
}

type ResolvedTaskBundle struct {
	Root          string
	RequiredFiles []TaskFile
	StatusFile    string
}

func (bundle ResolvedTaskBundle) StatusPath() string {
	return filepath.Join(bundle.Root, bundle.StatusFile)
}

type ThreadsProject struct {
	Name   string
	Source string
}

type StatusSnapshot struct {
	TaskID string
}

type Definition interface {
	Name() string
	TaskBundle(context Context, environ []string, now time.Time) (ResolvedTaskBundle, error)
	ReadStatus(path string) (StatusSnapshot, error)
	InitStatus(context Context, bundle ResolvedTaskBundle, now time.Time) ([]byte, StatusSnapshot, error)
	PulseStatus(path string, context Context, now time.Time) ([]byte, StatusSnapshot, error)
	EntrypointPath(context Context) string
	ResolveThreadsProject(context Context, environ []string, explicit string) ThreadsProject
}

type Resolved struct {
	Definition Definition
	Context    Context
	Config     Config
}

func Resolve(workingDirectory string, environ []string) (Resolved, error) {
	context, err := DetectContext(workingDirectory, environ)
	if err != nil {
		return Resolved{}, err
	}

	config, err := LoadConfig(context)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{
		Definition: Default{Config: config},
		Context:    context,
		Config:     config,
	}, nil
}

func DetectContext(workingDirectory string, environ []string) (Context, error) {
	repoContext, err := detectRepositoryContext(workingDirectory)
	if err != nil {
		return Context{}, err
	}

	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}

	scopePaths := []string{workingDirectory}
	if repoContext.RepositoryRoot != "" {
		scopePaths = []string{repoContext.RepositoryRoot}
	}

	return Context{
		WorkingDirectory: workingDirectory,
		WorkspaceKind:    repoContext.WorkspaceKind,
		RepositoryRoot:   repoContext.RepositoryRoot,
		RepositoryKind:   repoContext.RepositoryKind,
		GitBranch:        repoContext.GitBranch,
		ScopePaths:       scopePaths,
		ExecutionTarget:  detectExecutionTarget(environ),
		Host:             host,
	}, nil
}

type repositoryContext struct {
	WorkspaceKind  string
	RepositoryRoot string
	RepositoryKind string
	GitBranch      string
}

func detectExecutionTarget(environ []string) string {
	values := envMap(environ)
	if values["SSH_CONNECTION"] != "" || values["SSH_CLIENT"] != "" || values["SSH_TTY"] != "" {
		return "remote-ssh"
	}
	return "local"
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

func detectRepositoryContext(workingDirectory string) (repositoryContext, error) {
	current := workingDirectory
	for {
		gitPath := filepath.Join(current, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			repositoryKind := "git-repo-root"
			if !info.IsDir() {
				repositoryKind = "git-worktree"
			}

			workspaceKind := repositoryKind
			if workingDirectory != current {
				workspaceKind = "git-subdir"
			}

			return repositoryContext{
				WorkspaceKind:  workspaceKind,
				RepositoryRoot: current,
				RepositoryKind: repositoryKind,
				GitBranch:      readGitBranch(current),
			}, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			return repositoryContext{}, fmt.Errorf("stat %s: %w", gitPath, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return repositoryContext{
				WorkspaceKind: "plain-directory",
			}, nil
		}
		current = parent
	}
}

func readGitBranch(repoRoot string) string {
	gitDir, err := resolveGitDir(repoRoot)
	if err != nil {
		return ""
	}

	headPath := filepath.Join(gitDir, "HEAD")
	content, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}

	line := strings.TrimSpace(string(content))
	if strings.HasPrefix(line, "ref: ") {
		ref := strings.TrimPrefix(line, "ref: ")
		return strings.TrimPrefix(ref, "refs/heads/")
	}

	if line == "" {
		return ""
	}

	if len(line) > 12 {
		return fmt.Sprintf("detached:%s", line[:12])
	}
	return fmt.Sprintf("detached:%s", line)
}

func resolveGitDir(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}

	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("unsupported .git file format at %s", gitPath)
	}

	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}
	return filepath.Clean(filepath.Join(repoRoot, gitDir)), nil
}
