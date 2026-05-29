package workspace

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/artifact"
	"github.com/Andrew0613/Anton/internal/state"
)

type CockpitReport struct {
	Adapter          string             `json:"adapter"`
	WorkingDirectory string             `json:"working_directory"`
	RepositoryRoot   string             `json:"repository_root,omitempty"`
	ConfigPath       string             `json:"config_path"`
	ConfigSource     string             `json:"config_source"`
	Workspaces       []CockpitWorkspace `json:"workspaces"`
	Findings         []finding          `json:"findings"`
	Summary          CockpitSummary     `json:"summary"`
}

type CockpitSummary struct {
	Status               string `json:"status"`
	WorkspaceCount       int    `json:"workspace_count"`
	DirtyCount           int    `json:"dirty_count"`
	ResultHeavyCount     int    `json:"result_heavy_count"`
	CleanupReadyCount    int    `json:"cleanup_ready_count"`
	SplitBrainCount      int    `json:"split_brain_count"`
	InspectionBlocked    int    `json:"inspection_blocked_count"`
	LockedAttentionCount int    `json:"locked_attention_count"`
}

type CockpitWorkspace struct {
	Path              string             `json:"path"`
	Branch            string             `json:"branch,omitempty"`
	Current           bool               `json:"current"`
	Dirty             bool               `json:"dirty"`
	Locked            bool               `json:"locked"`
	Classification    string             `json:"classification"`
	RecommendedAction string             `json:"recommended_action"`
	ResultFootprint   artifact.Footprint `json:"result_footprint"`
}

func BuildCockpit(environ []string) (CockpitReport, error) {
	wd, err := os.Getwd()
	if err != nil {
		return CockpitReport{}, err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return CockpitReport{}, err
	}
	repoRoot := resolved.Context.RepositoryRoot
	if repoRoot == "" {
		repoRoot = resolved.Context.WorkingDirectory
	}
	currentRoot := strings.TrimSpace(gitCommand(wd, "rev-parse", "--show-toplevel"))
	if currentRoot == "" {
		currentRoot = wd
	}
	currentRoot = filepath.Clean(currentRoot)
	paths := workspacePaths(repoRoot)
	inventory, _ := state.LoadInventory(resolved, false)
	activeByPath := map[string]bool{}
	for _, item := range inventory.Active {
		if strings.TrimSpace(item.Workspace.Path) == "" {
			continue
		}
		activeByPath[normalizeWorkspacePath(repoRoot, item.Workspace.Path)] = true
	}
	workspaces := make([]CockpitWorkspace, 0, len(paths))
	for _, path := range paths {
		workspaces = append(workspaces, inspectWorkspace(path, repoRoot, currentRoot, activeByPath))
	}
	report := CockpitReport{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		RepositoryRoot:   resolved.Context.RepositoryRoot,
		ConfigPath:       resolved.Config.Path,
		ConfigSource:     resolved.Config.Source(),
		Workspaces:       workspaces,
		Findings:         []finding{},
	}
	if len(inventory.Active) > 1 {
		report.Findings = append(report.Findings, finding{
			Level:   "warning",
			Code:    "state-active-split-brain",
			Message: "multiple active tasks in state inventory; workspace truth is split",
		})
	}
	report.Summary = summarizeCockpit(workspaces, report.Findings)
	return report, nil
}

func workspacePaths(repoRoot string) []string {
	if paths := gitWorktreeListPaths(repoRoot); len(paths) > 0 {
		return paths
	}
	return metadataWorkspacePaths(repoRoot)
}

func gitWorktreeListPaths(repoRoot string) []string {
	output := gitCommand(repoRoot, "worktree", "list", "--porcelain")
	if strings.TrimSpace(output) == "" {
		return nil
	}
	paths := []string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if path != "" {
			paths = append(paths, filepath.Clean(path))
		}
	}
	return uniqueSortedPaths(paths)
}

func metadataWorkspacePaths(repoRoot string) []string {
	paths := []string{filepath.Clean(repoRoot)}
	gitDir := resolveGitDirForRefs(repoRoot)
	if gitDir == "" {
		return uniqueSortedPaths(paths)
	}
	commonDir := resolveCommonGitDir(gitDir)
	if filepath.Base(commonDir) == ".git" {
		paths = append(paths, filepath.Clean(filepath.Dir(commonDir)))
	}
	gitWorktrees := filepath.Join(commonDir, "worktrees")
	entries, err := os.ReadDir(gitWorktrees)
	if err != nil {
		return uniqueSortedPaths(paths)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		gitdirPath := filepath.Join(gitWorktrees, entry.Name(), "gitdir")
		content, err := os.ReadFile(gitdirPath)
		if err != nil {
			continue
		}
		worktreeGit := strings.TrimSpace(string(content))
		if worktreeGit == "" {
			continue
		}
		if !filepath.IsAbs(worktreeGit) {
			worktreeGit = filepath.Clean(filepath.Join(filepath.Dir(gitdirPath), worktreeGit))
		}
		workspacePath := filepath.Dir(worktreeGit)
		paths = append(paths, filepath.Clean(workspacePath))
	}
	return uniqueSortedPaths(paths)
}

func inspectWorkspace(path string, repoRoot string, currentWD string, activeByPath map[string]bool) CockpitWorkspace {
	path = filepath.Clean(path)
	current := path == filepath.Clean(currentWD)
	branch := strings.TrimSpace(gitCommand(path, "rev-parse", "--abbrev-ref", "HEAD"))
	statusOutput, statusOK := gitCommandStatus(path, "status", "--porcelain")
	dirty := strings.TrimSpace(statusOutput) != ""
	locked := statFile(indexLockPath(path))
	footprint := artifact.ScanResultFootprint(path)
	classification := "stale_clean"
	action := "keep"
	switch {
	case current:
		classification = "active_truth"
		action = "preserve-and-continue"
	case activeByPath[path]:
		classification = "active_truth"
		action = "preserve-and-continue"
	case locked:
		classification = "locked_attention"
		action = "resolve-lock-before-cleanup"
	case !statusOK:
		classification = "inspection_blocked"
		action = "inspect-before-cleanup"
	case footprint.FileCount > 0:
		classification = "result_persist_required"
		action = "record-artifact-retention-before-cleanup"
	case dirty && activeByPath[path]:
		classification = "active_truth"
		action = "preserve-and-continue"
	case dirty && (branch == "main" || branch == "master"):
		classification = "landed_dirty"
		action = "review-unexpected-dirty-main"
	case dirty:
		classification = "dirty_preserve"
		action = "review-before-cleanup"
	case branch == "main" || branch == "master":
		classification = "landed_clean"
		action = "cleanup_ready"
	case strings.HasPrefix(branch, "archive/"):
		classification = "archive_candidate"
		action = "archive-or-prune"
	default:
		classification = "cleanup_ready"
		action = "cleanup_ready"
	}
	return CockpitWorkspace{
		Path:              path,
		Branch:            branch,
		Current:           current,
		Dirty:             dirty,
		Locked:            locked,
		Classification:    classification,
		RecommendedAction: action,
		ResultFootprint:   footprint,
	}
}

func normalizeWorkspacePath(repoRoot string, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(repoRoot, path))
}

func uniqueSortedPaths(paths []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func indexLockPath(path string) string {
	gitDir := resolveGitDirForRefs(path)
	if gitDir == "" {
		return filepath.Join(path, ".git", "index.lock")
	}
	return filepath.Join(gitDir, "index.lock")
}

func summarizeCockpit(workspaces []CockpitWorkspace, findings []finding) CockpitSummary {
	result := CockpitSummary{
		Status:         "ok",
		WorkspaceCount: len(workspaces),
	}
	for _, item := range workspaces {
		if item.Dirty {
			result.DirtyCount++
		}
		if item.ResultFootprint.FileCount > 0 {
			result.ResultHeavyCount++
		}
		if item.Classification == "cleanup_ready" || item.Classification == "landed_clean" {
			result.CleanupReadyCount++
		}
		if item.Locked {
			result.LockedAttentionCount++
		}
		if item.Classification == "locked_attention" {
			result.Status = "blocked"
		}
		if item.Classification == "inspection_blocked" {
			result.InspectionBlocked++
			result.Status = "blocked"
		}
	}
	for _, finding := range findings {
		if finding.Code == "state-active-split-brain" {
			result.SplitBrainCount++
			if result.Status != "blocked" {
				result.Status = "degraded"
			}
		}
	}
	if result.Status == "ok" && (result.DirtyCount > 0 || result.ResultHeavyCount > 0) {
		result.Status = "degraded"
	}
	return result
}

func gitCommand(path string, args ...string) string {
	output, _ := gitCommandStatus(path, args...)
	return output
}

func gitCommandStatus(path string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if ctx.Err() != nil {
		return "", false
	}
	if err != nil {
		return "", false
	}
	return string(output), true
}

func statFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
