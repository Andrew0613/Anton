package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	paths := workspacePaths(repoRoot)
	inventory, _ := state.LoadInventory(resolved, false)
	activeByPath := map[string]bool{}
	for _, item := range inventory.Active {
		if strings.TrimSpace(item.Workspace.Path) == "" {
			continue
		}
		activeByPath[filepath.Clean(filepath.Join(repoRoot, item.Workspace.Path))] = true
	}
	workspaces := make([]CockpitWorkspace, 0, len(paths))
	for _, path := range paths {
		workspaces = append(workspaces, inspectWorkspace(path, repoRoot, wd, activeByPath))
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
	paths := []string{filepath.Clean(repoRoot)}
	gitWorktrees := filepath.Join(repoRoot, ".git", "worktrees")
	entries, err := os.ReadDir(gitWorktrees)
	if err != nil {
		return paths
	}
	for _, entry := range entries {
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
	return paths
}

func inspectWorkspace(path string, repoRoot string, currentWD string, activeByPath map[string]bool) CockpitWorkspace {
	branch := strings.TrimSpace(gitCommand(path, "rev-parse", "--abbrev-ref", "HEAD"))
	dirty := strings.TrimSpace(gitCommand(path, "status", "--porcelain")) != ""
	locked := statFile(filepath.Join(path, ".git", "index.lock"))
	footprint := artifact.ScanResultFootprint(path)
	classification := "stale_clean"
	action := "keep"
	switch {
	case locked:
		classification = "locked_attention"
		action = "resolve-lock-before-cleanup"
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
	case activeByPath[path]:
		classification = "active_truth"
		action = "preserve-and-continue"
	default:
		classification = "cleanup_ready"
		action = "cleanup_ready"
	}
	return CockpitWorkspace{
		Path:              path,
		Branch:            branch,
		Current:           filepath.Clean(path) == filepath.Clean(currentWD) || strings.HasPrefix(filepath.Clean(currentWD), filepath.Clean(path)+string(filepath.Separator)),
		Dirty:             dirty,
		Locked:            locked,
		Classification:    classification,
		RecommendedAction: action,
		ResultFootprint:   footprint,
	}
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
		if item.Classification == "locked_attention" {
			result.LockedAttentionCount++
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
	command := exec.Command("git", append([]string{"-C", path}, args...)...)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func statFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
