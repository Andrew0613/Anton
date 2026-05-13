package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const maxReferenceScanBytes int64 = 1024 * 1024

var referenceFileKinds = map[string]string{
	".md":       "markdown",
	".markdown": "markdown",
	".yaml":     "yaml",
	".yml":      "yaml",
	".go":       "go",
	".sh":       "shell",
	".bash":     "shell",
	".zsh":      "shell",
}

var skippedReferenceRoots = map[string]string{
	".git":         "git-metadata",
	".worktrees":   "git-worktree-root",
	".cache":       "generated-or-cache-root",
	"bin":          "generated-or-build-root",
	"build":        "generated-or-build-root",
	"coverage":     "generated-or-build-root",
	"dist":         "generated-or-build-root",
	"node_modules": "dependency-root",
	"out":          "generated-or-build-root",
	"vendor":       "dependency-root",
}

type RefsReport struct {
	Adapter          string              `json:"adapter"`
	WorkingDirectory string              `json:"working_directory"`
	RepositoryRoot   string              `json:"repository_root"`
	ConfigPath       string              `json:"config_path"`
	ConfigSource     string              `json:"config_source"`
	Target           TargetStatus        `json:"target"`
	ReferenceHits    []ReferenceHit      `json:"reference_hits"`
	SkippedRoots     []SkippedRoot       `json:"skipped_roots"`
	Worktrees        []WorktreeOccupancy `json:"worktrees"`
	TaskBundle       TaskBundleStatus    `json:"task_bundle"`
	AffectedSurfaces []AffectedSurface   `json:"affected_surfaces"`
	Findings         []RefFinding        `json:"findings"`
	Summary          RefsSummary         `json:"summary"`
	ReadOnly         bool                `json:"read_only"`
}

type TargetStatus struct {
	Requested          string `json:"requested"`
	Normalized         string `json:"normalized"`
	Relative           string `json:"relative"`
	Exists             bool   `json:"exists"`
	BoundaryStatus     string `json:"boundary_status"`
	SymlinkStatus      string `json:"symlink_status"`
	ResolvedExisting   string `json:"resolved_existing,omitempty"`
	ResolvedExistingIs string `json:"resolved_existing_is,omitempty"`
}

type ReferenceHit struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Matched string `json:"matched"`
}

type SkippedRoot struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type WorktreeOccupancy struct {
	Path           string `json:"path"`
	Branch         string `json:"branch,omitempty"`
	Current        bool   `json:"current"`
	OverlapsTarget bool   `json:"overlaps_target"`
}

type TaskBundleStatus struct {
	Root           string `json:"root"`
	Absolute       string `json:"absolute"`
	TargetOverlaps bool   `json:"target_overlaps"`
	StatusPath     string `json:"status_path,omitempty"`
	State          string `json:"state,omitempty"`
	Message        string `json:"message,omitempty"`
}

type AffectedSurface struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type RefFinding struct {
	Level       string `json:"level"`
	Code        string `json:"code"`
	Path        string `json:"path,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type RefsSummary struct {
	Status         string `json:"status"`
	Blockers       int    `json:"blockers"`
	Warnings       int    `json:"warnings"`
	ReferenceHits  int    `json:"reference_hits"`
	SkippedRoots   int    `json:"skipped_roots"`
	Recommendation string `json:"recommendation"`
}

func BuildRefsReport(environ []string, target string) (RefsReport, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return RefsReport{}, fmt.Errorf("target is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return RefsReport{}, err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return RefsReport{}, err
	}
	repoRoot := resolved.Context.RepositoryRoot
	if repoRoot == "" {
		repoRoot = resolved.Context.WorkingDirectory
	}
	repoRoot = filepath.Clean(repoRoot)

	report := RefsReport{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		RepositoryRoot:   repoRoot,
		ConfigPath:       resolved.Config.Path,
		ConfigSource:     resolved.Config.Source(),
		ReadOnly:         true,
	}
	report.Target = resolveTargetStatus(repoRoot, target)
	report.TaskBundle = inspectTaskBundleTarget(repoRoot, resolved.Config.Tasks.Root, report.Target.Normalized)
	report.Worktrees = inspectWorktrees(repoRoot, report.Target.Normalized)

	if report.Target.BoundaryStatus == "blocked" {
		report.Findings = append(report.Findings, RefFinding{
			Level:       "error",
			Code:        "target-escapes-repo",
			Path:        report.Target.Normalized,
			Message:     "target path escapes the repository boundary",
			Remediation: "choose a target path inside the repository root",
		})
	} else if report.Target.SymlinkStatus == "blocked" {
		report.Findings = append(report.Findings, RefFinding{
			Level:       "error",
			Code:        "target-symlink-escape",
			Path:        report.Target.Normalized,
			Message:     "target path resolves through a symlink outside the repository boundary",
			Remediation: "replace the symlink or choose a repo-local target",
		})
	} else {
		report.ReferenceHits, report.SkippedRoots = scanReferenceHits(repoRoot, report.Target.Relative)
		report.AffectedSurfaces = inferAffectedSurfaces(report.Target.Relative, report.ReferenceHits)
	}

	if len(report.ReferenceHits) > 0 {
		report.Findings = append(report.Findings, RefFinding{
			Level:       "warning",
			Code:        "target-references-found",
			Path:        report.Target.Relative,
			Message:     fmt.Sprintf("found %d textual reference(s) to the target", len(report.ReferenceHits)),
			Remediation: "review reference hits before renaming or moving the target",
		})
	}
	if len(report.SkippedRoots) > 0 {
		report.Findings = append(report.Findings, RefFinding{
			Level:       "warning",
			Code:        "reference-roots-skipped",
			Message:     fmt.Sprintf("skipped %d ignored/generated root(s) during reference scan", len(report.SkippedRoots)),
			Remediation: "scan skipped roots explicitly if generated or vendored references matter",
		})
	}
	for _, worktree := range report.Worktrees {
		if worktree.OverlapsTarget && !worktree.Current {
			report.Findings = append(report.Findings, RefFinding{
				Level:       "warning",
				Code:        "target-overlaps-worktree",
				Path:        worktree.Path,
				Message:     "target overlaps another git worktree",
				Remediation: "coordinate with that worktree before moving shared paths",
			})
		}
	}
	if report.TaskBundle.TargetOverlaps {
		report.Findings = append(report.Findings, RefFinding{
			Level:       "warning",
			Code:        "target-overlaps-task-root",
			Path:        report.TaskBundle.Absolute,
			Message:     "target overlaps the configured Anton task root",
			Remediation: "preserve task bundle state or retarget it explicitly before moving paths",
		})
	}

	report.Summary = summarizeRefs(report)
	return report, nil
}

func resolveTargetStatus(repoRoot string, target string) TargetStatus {
	normalized := target
	if !filepath.IsAbs(normalized) {
		normalized = filepath.Join(repoRoot, normalized)
	}
	normalized = filepath.Clean(normalized)
	relative, relErr := filepath.Rel(repoRoot, normalized)
	status := TargetStatus{
		Requested:      target,
		Normalized:     normalized,
		Relative:       filepath.ToSlash(relative),
		BoundaryStatus: "ok",
		SymlinkStatus:  "ok",
	}
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		status.Relative = ""
		status.BoundaryStatus = "blocked"
		status.SymlinkStatus = "skipped"
		return status
	}
	if info, err := os.Stat(normalized); err == nil {
		status.Exists = true
		status.ResolvedExistingIs = "target"
		if info.IsDir() {
			status.ResolvedExistingIs = "target-directory"
		}
	} else if !os.IsNotExist(err) {
		status.SymlinkStatus = "blocked"
		return status
	}

	existing := normalized
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	if real, err := filepath.EvalSymlinks(existing); err == nil {
		status.ResolvedExisting = filepath.Clean(real)
		if !pathWithinRoot(repoRoot, status.ResolvedExisting) {
			status.SymlinkStatus = "blocked"
		}
	}
	return status
}

func scanReferenceHits(repoRoot string, targetRelative string) ([]ReferenceHit, []SkippedRoot) {
	hits := []ReferenceHit{}
	skipped := []SkippedRoot{}
	terms := referenceTerms(targetRelative)

	_ = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == repoRoot {
			return nil
		}
		relative, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		relativeSlash := filepath.ToSlash(relative)
		if entry.IsDir() {
			if reason, ok := skippedReferenceRoots[entry.Name()]; ok {
				skipped = append(skipped, SkippedRoot{Path: relativeSlash, Reason: reason})
				return filepath.SkipDir
			}
			return nil
		}
		kind, ok := referenceFileKinds[strings.ToLower(filepath.Ext(entry.Name()))]
		if !ok {
			return nil
		}
		fileHits := scanFileForTerms(path, relativeSlash, kind, terms)
		hits = append(hits, fileHits...)
		return nil
	})
	return hits, skipped
}

func referenceTerms(targetRelative string) []string {
	targetRelative = strings.Trim(filepath.ToSlash(targetRelative), "/")
	if targetRelative == "" || targetRelative == "." {
		return []string{}
	}
	terms := []string{targetRelative, "./" + targetRelative}
	if dir := filepath.ToSlash(filepath.Dir(targetRelative)); dir != "." {
		terms = append(terms, dir+"/"+filepath.Base(targetRelative))
	}
	return uniqueStrings(terms)
}

func scanFileForTerms(path string, relative string, kind string, terms []string) []ReferenceHit {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxReferenceScanBytes {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	hits := []ReferenceHit{}
	reader := bufio.NewReader(file)
	lineNumber := 0
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			lineNumber++
			for _, term := range terms {
				if term == "" {
					continue
				}
				column := strings.Index(line, term)
				if column >= 0 {
					hits = append(hits, ReferenceHit{
						Path:    relative,
						Kind:    kind,
						Line:    lineNumber,
						Column:  column + 1,
						Matched: term,
					})
					break
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	return hits
}

func inspectTaskBundleTarget(repoRoot string, configuredRoot string, target string) TaskBundleStatus {
	absolute := configuredRoot
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(repoRoot, configuredRoot)
	}
	absolute = filepath.Clean(absolute)
	status := TaskBundleStatus{
		Root:     configuredRoot,
		Absolute: absolute,
	}
	if pathWithinRoot(absolute, target) || pathWithinRoot(target, absolute) {
		status.TargetOverlaps = true
		status.Message = "target overlaps configured task root"
		if statusPath := nearestStatusYAML(absolute, target); statusPath != "" {
			status.StatusPath = statusPath
			status.State = readStatusState(statusPath)
		}
	}
	return status
}

func nearestStatusYAML(root string, target string) string {
	current := target
	if info, err := os.Stat(current); err == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}
	for pathWithinRoot(root, current) {
		candidate := filepath.Join(current, "status.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func readStatusState(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "status:") || strings.HasPrefix(line, "state:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return ""
}

func inspectWorktrees(repoRoot string, target string) []WorktreeOccupancy {
	worktrees := []WorktreeOccupancy{{
		Path:           repoRoot,
		Branch:         readBranch(filepath.Join(repoRoot, ".git", "HEAD")),
		Current:        true,
		OverlapsTarget: pathsOverlap(repoRoot, target),
	}}
	gitDir := resolveGitDirForRefs(repoRoot)
	if gitDir == "" {
		return worktrees
	}
	commonDir := resolveCommonGitDir(gitDir)
	entries, err := os.ReadDir(filepath.Join(commonDir, "worktrees"))
	if err != nil {
		return worktrees
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadataDir := filepath.Join(commonDir, "worktrees", entry.Name())
		gitdirPath := strings.TrimSpace(readSmallFile(filepath.Join(metadataDir, "gitdir")))
		if gitdirPath == "" {
			continue
		}
		path := filepath.Dir(gitdirPath)
		worktrees = append(worktrees, WorktreeOccupancy{
			Path:           filepath.Clean(path),
			Branch:         readBranch(filepath.Join(metadataDir, "HEAD")),
			Current:        false,
			OverlapsTarget: pathsOverlap(path, target),
		})
	}
	return worktrees
}

func resolveGitDirForRefs(repoRoot string) string {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err == nil && info.IsDir() {
		return gitPath
	}
	content := strings.TrimSpace(readSmallFile(gitPath))
	if strings.HasPrefix(content, "gitdir:") {
		value := strings.TrimSpace(strings.TrimPrefix(content, "gitdir:"))
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
		return filepath.Clean(filepath.Join(repoRoot, value))
	}
	return ""
}

func resolveCommonGitDir(gitDir string) string {
	content := strings.TrimSpace(readSmallFile(filepath.Join(gitDir, "commondir")))
	if content == "" {
		return gitDir
	}
	if filepath.IsAbs(content) {
		return filepath.Clean(content)
	}
	return filepath.Clean(filepath.Join(gitDir, content))
}

func readBranch(headPath string) string {
	content := strings.TrimSpace(readSmallFile(headPath))
	if strings.HasPrefix(content, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(content, "ref:"))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if len(content) >= 7 {
		return content[:7]
	}
	return content
}

func readSmallFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil || len(content) > 64*1024 {
		return ""
	}
	return string(content)
}

func pathsOverlap(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return pathWithinRoot(left, right) || pathWithinRoot(right, left)
}

func inferAffectedSurfaces(targetRelative string, hits []ReferenceHit) []AffectedSurface {
	seen := map[string]string{}
	add := func(name string, reason string) {
		if _, ok := seen[name]; !ok {
			seen[name] = reason
		}
	}
	if strings.HasPrefix(targetRelative, ".github/workflows/") {
		add("github-actions", "target is under .github/workflows")
	}
	for _, hit := range hits {
		if strings.HasPrefix(hit.Path, ".github/workflows/") {
			add("github-actions", "reference appears in a GitHub Actions workflow")
		}
		switch hit.Kind {
		case "go":
			add("go-checks", "reference appears in Go source")
		case "shell":
			add("shell-scripts", "reference appears in shell script")
		case "yaml":
			add("config", "reference appears in YAML config")
		case "markdown":
			add("docs", "reference appears in Markdown docs")
		}
	}
	surfaces := make([]AffectedSurface, 0, len(seen))
	for name, reason := range seen {
		surfaces = append(surfaces, AffectedSurface{Name: name, Reason: reason})
	}
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].Name < surfaces[j].Name })
	return surfaces
}

func summarizeRefs(report RefsReport) RefsSummary {
	summary := RefsSummary{
		Status:         "ok",
		ReferenceHits:  len(report.ReferenceHits),
		SkippedRoots:   len(report.SkippedRoots),
		Recommendation: "go",
	}
	for _, finding := range report.Findings {
		switch finding.Level {
		case "error":
			summary.Blockers++
		case "warning":
			summary.Warnings++
		}
	}
	if summary.Blockers > 0 {
		summary.Status = "blocked"
		summary.Recommendation = "no-go"
	} else if summary.Warnings > 0 {
		summary.Status = "degraded"
		summary.Recommendation = "go-with-caution"
	}
	return summary
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
