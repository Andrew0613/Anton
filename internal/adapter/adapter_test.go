package adapter

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDetectContextRepoRootFixture(t *testing.T) {
	path := fixturePath(t, "repo-root")

	context, err := DetectContext(path, nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	if context.WorkspaceKind != "git-repo-root" {
		t.Fatalf("workspace kind = %q", context.WorkspaceKind)
	}
	if context.RepositoryKind != "git-repo-root" {
		t.Fatalf("repository kind = %q", context.RepositoryKind)
	}
	if context.RepositoryRoot != path {
		t.Fatalf("repository root = %q, want %q", context.RepositoryRoot, path)
	}
	if context.GitBranch != "main" {
		t.Fatalf("git branch = %q", context.GitBranch)
	}
	if len(context.ScopePaths) != 1 || context.ScopePaths[0] != path {
		t.Fatalf("scope paths = %#v", context.ScopePaths)
	}
}

func TestDetectContextRepoSubdirFixture(t *testing.T) {
	path := filepath.Join(fixturePath(t, "repo-root"), "subdir")

	context, err := DetectContext(path, []string{"SSH_CONNECTION=fixture"})
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	if context.WorkspaceKind != "git-subdir" {
		t.Fatalf("workspace kind = %q", context.WorkspaceKind)
	}
	if context.RepositoryRoot != fixturePath(t, "repo-root") {
		t.Fatalf("repository root = %q", context.RepositoryRoot)
	}
	if context.ExecutionTarget != "remote-ssh" {
		t.Fatalf("execution target = %q", context.ExecutionTarget)
	}
}

func TestDetectContextWorktreeFixture(t *testing.T) {
	path := fixturePath(t, "worktree")

	context, err := DetectContext(path, nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	if context.WorkspaceKind != "git-worktree" {
		t.Fatalf("workspace kind = %q", context.WorkspaceKind)
	}
	if context.RepositoryKind != "git-worktree" {
		t.Fatalf("repository kind = %q", context.RepositoryKind)
	}
	if context.GitBranch != "task/anton-bootstrap" {
		t.Fatalf("git branch = %q", context.GitBranch)
	}
}

func TestDetectContextPlainDirectoryFixture(t *testing.T) {
	path := fixturePath(t, "plain-directory")

	context, err := DetectContext(path, nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	if context.WorkspaceKind != "plain-directory" {
		t.Fatalf("workspace kind = %q", context.WorkspaceKind)
	}
	if context.RepositoryRoot != "" {
		t.Fatalf("repository root = %q", context.RepositoryRoot)
	}
	if len(context.ScopePaths) != 1 || context.ScopePaths[0] != path {
		t.Fatalf("scope paths = %#v", context.ScopePaths)
	}
}

func TestLoadConfigDefaultsWhenAntonYAMLIsMissing(t *testing.T) {
	context, err := DetectContext(fixturePath(t, "repo-root"), nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	config, err := LoadConfig(context)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.Tasks.Root != ".anton/tasks" {
		t.Fatalf("tasks root = %q", config.Tasks.Root)
	}
	if config.Entrypoint.Path != "AGENTS.md" {
		t.Fatalf("entrypoint path = %q", config.Entrypoint.Path)
	}
	if config.Threads.DefaultProjectStrategy != "repo-root" {
		t.Fatalf("project strategy = %q", config.Threads.DefaultProjectStrategy)
	}
	if config.Loaded {
		t.Fatalf("config should not report Loaded=true when anton.yaml is missing")
	}
	wantPath := filepath.Join(fixturePath(t, "repo-root"), "anton.yaml")
	if config.Path != wantPath {
		t.Fatalf("config path = %q, want %q", config.Path, wantPath)
	}
}

func TestResolveLoadsConfiguredCanonicalAdapter(t *testing.T) {
	resolved, err := Resolve(fixturePath(t, "configured-repo"), nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Definition.Name() != "canonical" {
		t.Fatalf("adapter = %q", resolved.Definition.Name())
	}
	if resolved.Config.Tasks.Root != ".anton/tasks" {
		t.Fatalf("tasks root = %q", resolved.Config.Tasks.Root)
	}
	if resolved.Config.Entrypoint.Path != "ops/AGENTS.md" {
		t.Fatalf("entrypoint path = %q", resolved.Config.Entrypoint.Path)
	}
	if !resolved.Config.Loaded {
		t.Fatalf("resolved config should report Loaded=true when anton.yaml exists")
	}
}

func TestDefaultTaskBundleUsesConfiguredCanonicalRoot(t *testing.T) {
	context, err := DetectContext(fixturePath(t, "configured-repo"), nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	definition := Default{Config: mustLoadConfig(t, context)}
	bundle, err := definition.TaskBundle(context, nil, time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("TaskBundle returned error: %v", err)
	}
	if bundle.StatusFile != "status.yaml" {
		t.Fatalf("status file = %q", bundle.StatusFile)
	}
	wantRoot := filepath.Join(fixturePath(t, "configured-repo"), ".anton", "tasks", "active", "demo_task")
	if bundle.Root != wantRoot {
		t.Fatalf("bundle root = %q, want %q", bundle.Root, wantRoot)
	}
	if len(bundle.RequiredFiles) != 3 {
		t.Fatalf("required files = %d", len(bundle.RequiredFiles))
	}
	if bundle.RequiredFiles[0].Name != "task_plan.md" || bundle.RequiredFiles[1].Name != "findings.md" || bundle.RequiredFiles[2].Name != "progress.md" {
		t.Fatalf("unexpected task bundle order: %#v", bundle.RequiredFiles)
	}
}

func TestDefaultResolveThreadsProject(t *testing.T) {
	context := Context{
		WorkingDirectory: fixturePath(t, "repo-root"),
		RepositoryRoot:   fixturePath(t, "repo-root"),
	}

	flagProject := Default{}.ResolveThreadsProject(context, nil, "Anton")
	if flagProject.Name != "Anton" || flagProject.Source != "flag" {
		t.Fatalf("flag project = %#v", flagProject)
	}

	envProject := Default{}.ResolveThreadsProject(context, []string{"ANTON_THREADS_PROJECT=physedit"}, "")
	if envProject.Name != "physedit" || envProject.Source != "env" {
		t.Fatalf("env project = %#v", envProject)
	}

	repoProject := Default{}.ResolveThreadsProject(context, nil, "")
	if repoProject.Name != "repo-root" || repoProject.Source != "repo-root" {
		t.Fatalf("repo project = %#v", repoProject)
	}
}

func TestDefaultEntrypointPathRespectsConfig(t *testing.T) {
	repoRoot := fixturePath(t, "configured-repo")
	context := Context{
		WorkingDirectory: filepath.Join(repoRoot, "subdir"),
		RepositoryRoot:   repoRoot,
	}

	path := Default{Config: mustLoadConfig(t, context)}.EntrypointPath(context)
	want := filepath.Join(repoRoot, "ops", "AGENTS.md")
	if path != want {
		t.Fatalf("entrypoint path = %q, want %q", path, want)
	}
}

func TestTaskBundleUsesCurrentCanonicalBundleWhenAlreadyInsideTaskDirectory(t *testing.T) {
	taskDir := filepath.Join(fixturePath(t, "configured-repo"), ".anton", "tasks", "active", "demo_task")
	context, err := DetectContext(taskDir, nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	bundle, err := Default{Config: mustLoadConfig(t, context)}.TaskBundle(context, nil, time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("TaskBundle returned error: %v", err)
	}
	if bundle.Root != taskDir {
		t.Fatalf("bundle root = %q, want %q", bundle.Root, taskDir)
	}
}

func TestConfiguredWorkspaceRootInfersThreadsProject(t *testing.T) {
	context, err := DetectContext(filepath.Join(fixturePath(t, "configured-repo"), ".anton", "workspaces", "ISSUE-42"), nil)
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}

	project := Default{Config: mustLoadConfig(t, context)}.ResolveThreadsProject(context, nil, "")
	if project.Name != "ISSUE-42" || project.Source != "workspace-root" {
		t.Fatalf("project = %#v", project)
	}
}

func TestCanonicalStatusReadParsesConfiguredFixture(t *testing.T) {
	path := filepath.Join(fixturePath(t, "configured-repo"), ".anton", "tasks", "active", "demo_task", "status.yaml")

	snapshot, err := Default{}.ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}
	if snapshot.TaskID != "demo_task" {
		t.Fatalf("task id = %q", snapshot.TaskID)
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "contexts", name)
}

func mustLoadConfig(t *testing.T, context Context) Config {
	t.Helper()
	config, err := LoadConfig(context)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	return config
}
