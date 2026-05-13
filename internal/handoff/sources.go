package handoff

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	maxSourceFiles     = 500
	maxSourceLineBytes = 64 * 1024
	maxSourceSnippets  = 8
	maxPersistFiles    = 200
)

type taskStatusSummary struct {
	TaskID               string            `json:"task_id"`
	Lifecycle            string            `json:"lifecycle"`
	FinishState          string            `json:"finish_state"`
	NextStep             string            `json:"next_step,omitempty"`
	Blockers             []string          `json:"blockers,omitempty"`
	ExpectedDeliverables []string          `json:"expected_deliverables,omitempty"`
	Attempts             []handoffEvidence `json:"attempts,omitempty"`
	ValidationReceipts   []handoffEvidence `json:"validation_receipts,omitempty"`
}

type handoffEvidence struct {
	Command   string   `json:"command,omitempty" yaml:"command"`
	At        string   `json:"at,omitempty" yaml:"at"`
	Outcome   string   `json:"outcome,omitempty" yaml:"outcome"`
	Validated bool     `json:"validated" yaml:"validated"`
	Artifacts []string `json:"artifacts,omitempty" yaml:"artifacts"`
	Notes     string   `json:"notes,omitempty" yaml:"notes"`
}

type gitSummary struct {
	Branch          string   `json:"branch,omitempty"`
	ShortSHA        string   `json:"short_sha,omitempty"`
	DirtyTracked    []string `json:"dirty_tracked,omitempty"`
	Untracked       []string `json:"untracked,omitempty"`
	StatusAvailable bool     `json:"status_available"`
	Warnings        []string `json:"warnings,omitempty"`
}

type sourceSummary struct {
	Kind      string `json:"kind"`
	SessionID string `json:"session_id,omitempty"`
	Path      string `json:"path,omitempty"`
}

type sourceSnippet struct {
	Source    string `json:"source"`
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Text      string `json:"text"`
}

type persistPlan struct {
	DryRun          bool            `json:"dry_run"`
	WouldWrite      bool            `json:"would_write"`
	WorktreeRoot    string          `json:"worktree_root"`
	RunDir          string          `json:"run_dir"`
	DestinationRoot string          `json:"destination_root"`
	FileCount       int             `json:"file_count"`
	ByteCount       int64           `json:"byte_count"`
	Copies          []copyPlanEntry `json:"copies,omitempty"`
	Skipped         []string        `json:"skipped,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type copyPlanEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Bytes       int64  `json:"bytes"`
}

type statusYAML struct {
	Stable struct {
		TaskID string `yaml:"task_id"`
	} `yaml:"stable"`
	State struct {
		Lifecycle string `yaml:"lifecycle"`
	} `yaml:"state"`
	Evidence struct {
		Attempts    []handoffEvidence `yaml:"attempts"`
		Validations []handoffEvidence `yaml:"validations"`
	} `yaml:"evidence"`
	Closure struct {
		FinishState          string   `yaml:"finish_state"`
		NextStep             string   `yaml:"next_step"`
		Blockers             []string `yaml:"blockers"`
		ExpectedDeliverables []string `yaml:"expected_deliverables"`
	} `yaml:"closure"`
}

var handoffRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|authorization)\s*[:=]\s*["']?[^"'\s,}]+`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`hf_[A-Za-z0-9]{16,}`),
}

func readTaskStatusSummary(path string, snapshot adapter.StatusSnapshot) (taskStatusSummary, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return taskStatusSummary{}, fmt.Errorf("read %s: %w", path, err)
	}
	var status statusYAML
	if err := yaml.Unmarshal(content, &status); err != nil {
		return taskStatusSummary{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if strings.TrimSpace(status.Stable.TaskID) == "" ||
		strings.TrimSpace(status.State.Lifecycle) == "" ||
		strings.TrimSpace(status.Closure.FinishState) == "" {
		return taskStatusSummaryFromSnapshot(snapshot), nil
	}
	return taskStatusSummary{
		TaskID:               strings.TrimSpace(status.Stable.TaskID),
		Lifecycle:            strings.TrimSpace(status.State.Lifecycle),
		FinishState:          strings.TrimSpace(status.Closure.FinishState),
		NextStep:             strings.TrimSpace(status.Closure.NextStep),
		Blockers:             trimList(status.Closure.Blockers),
		ExpectedDeliverables: trimList(status.Closure.ExpectedDeliverables),
		Attempts:             status.Evidence.Attempts,
		ValidationReceipts:   status.Evidence.Validations,
	}, nil
}

func taskStatusSummaryFromSnapshot(snapshot adapter.StatusSnapshot) taskStatusSummary {
	return taskStatusSummary{
		TaskID:      strings.TrimSpace(snapshot.TaskID),
		Lifecycle:   strings.TrimSpace(snapshot.Lifecycle),
		FinishState: strings.TrimSpace(snapshot.FinishState),
		NextStep:    strings.TrimSpace(snapshot.NextStep),
	}
}

func collectGitSummary(repoRoot string, fallbackBranch string) gitSummary {
	git := gitSummary{Branch: strings.TrimSpace(fallbackBranch)}
	if strings.TrimSpace(repoRoot) == "" {
		git.Warnings = append(git.Warnings, "git repository root unavailable")
		return git
	}

	if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" && branch != "HEAD" {
			git.Branch = branch
		}
	}
	if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD").Output(); err == nil {
		git.ShortSHA = strings.TrimSpace(string(out))
	} else {
		git.Warnings = append(git.Warnings, "git short SHA unavailable")
	}
	out, err := exec.Command("git", "-C", repoRoot, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		git.Warnings = append(git.Warnings, "git status unavailable")
		return git
	}
	git.StatusAvailable = true
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "?? ") {
			git.Untracked = append(git.Untracked, strings.TrimSpace(strings.TrimPrefix(line, "?? ")))
			continue
		}
		if len(line) >= 4 {
			git.DirtyTracked = append(git.DirtyTracked, strings.TrimSpace(line[3:]))
		}
	}
	sort.Strings(git.DirtyTracked)
	sort.Strings(git.Untracked)
	return git
}

func collectSourceSnippets(source string, sessionID string, environ []string) (sourceSummary, []sourceSnippet, []string, error) {
	summary := sourceSummary{Kind: firstNonEmpty(strings.TrimSpace(source), "manual")}
	if summary.Kind == "manual" {
		return summary, nil, nil, nil
	}
	summary.SessionID = strings.TrimSpace(sessionID)
	path, warnings, err := locateSourceSession(summary.Kind, summary.SessionID, environ)
	if err != nil {
		return summary, nil, warnings, err
	}
	summary.Path = path
	snippets, readWarnings, err := readSourceSnippets(summary.Kind, summary.SessionID, path)
	warnings = append(warnings, readWarnings...)
	return summary, snippets, warnings, err
}

func locateSourceSession(source string, sessionID string, environ []string) (string, []string, error) {
	if sessionID == "" {
		return "", nil, fmt.Errorf("--session-id is required when --source is %s", source)
	}
	if info, err := os.Stat(sessionID); err == nil && !info.IsDir() {
		path, absErr := filepath.Abs(sessionID)
		if absErr != nil {
			return sessionID, nil, nil
		}
		return path, nil, nil
	}

	roots := sourceRoots(source, environ)
	var warnings []string
	for _, root := range roots {
		path, limited, err := findSessionPath(root, sessionID)
		if limited {
			warnings = append(warnings, fmt.Sprintf("source search under %s was limited to %d files", root, maxSourceFiles))
		}
		if err == nil && path != "" {
			return path, warnings, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			warnings = append(warnings, err.Error())
		}
	}
	return "", warnings, fmt.Errorf("%s session %q not found", source, sessionID)
}

func sourceRoots(source string, environ []string) []string {
	env := envMap(environ)
	var roots []string
	if root := strings.TrimSpace(env["ANTON_HANDOFF_SOURCE_ROOT"]); root != "" {
		roots = append(roots, root)
	}
	home := strings.TrimSpace(env["HOME"])
	switch source {
	case "codex":
		if codexHome := strings.TrimSpace(env["CODEX_HOME"]); codexHome != "" {
			roots = append(roots, filepath.Join(codexHome, "sessions"))
		}
		if home != "" {
			roots = append(roots, filepath.Join(home, ".codex", "sessions"))
		}
	case "claude":
		if claudeHome := strings.TrimSpace(env["CLAUDE_HOME"]); claudeHome != "" {
			roots = append(roots, filepath.Join(claudeHome, "projects"))
		}
		if home != "" {
			roots = append(roots, filepath.Join(home, ".claude", "projects"))
		}
	}
	return roots
}

func findSessionPath(root string, sessionID string) (string, bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("source root is not a directory: %s", root)
	}

	count := 0
	var contentMatch string
	var limited bool
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" && filepath.Ext(path) != ".json" && filepath.Ext(path) != ".md" && filepath.Ext(path) != ".txt" {
			return nil
		}
		count++
		if count > maxSourceFiles {
			limited = true
			return filepath.SkipAll
		}
		if strings.Contains(filepath.Base(path), sessionID) {
			contentMatch = path
			return filepath.SkipAll
		}
		content, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(content), sessionID) {
			contentMatch = path
			return filepath.SkipAll
		}
		return nil
	})
	if contentMatch != "" {
		return contentMatch, limited, nil
	}
	return "", limited, err
}

func readSourceSnippets(source string, sessionID string, path string) ([]sourceSnippet, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read source session %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxSourceLineBytes)
	var candidates []sourceSnippet
	var fallback []sourceSnippet
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		snippet := sourceSnippet{
			Source:    source,
			SessionID: sessionID,
			Path:      path,
			Line:      lineNumber,
			Text:      compact(redactHandoffText(text)),
		}
		if len(fallback) >= maxSourceSnippets {
			copy(fallback, fallback[1:])
			fallback[len(fallback)-1] = snippet
		} else {
			fallback = append(fallback, snippet)
		}
		if isSourceSnippetCandidate(text) {
			candidates = append(candidates, snippet)
			if len(candidates) >= maxSourceSnippets {
				break
			}
		}
	}
	var warnings []string
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("source scan stopped early: %v", err))
	}
	if len(candidates) > 0 {
		return candidates, warnings, nil
	}
	return fallback, warnings, nil
}

func buildPersistPlan(worktreeRoot string, runDir string, dryRun bool) (persistPlan, error) {
	root, err := cleanExistingDir(worktreeRoot, "--worktree-root")
	if err != nil {
		return persistPlan{}, err
	}
	run, err := cleanExistingDir(runDir, "--run-dir")
	if err != nil {
		return persistPlan{}, err
	}

	destinationRoot := filepath.Join(root, ".anton", "handoff", "results", filepath.Base(run))
	plan := persistPlan{
		DryRun:          dryRun,
		WouldWrite:      false,
		WorktreeRoot:    root,
		RunDir:          run,
		DestinationRoot: destinationRoot,
	}
	err = filepath.WalkDir(run, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			plan.Warnings = append(plan.Warnings, err.Error())
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(plan.Copies) >= maxPersistFiles {
			plan.Skipped = append(plan.Skipped, fmt.Sprintf("copy plan limited to %d files", maxPersistFiles))
			return filepath.SkipAll
		}
		info, statErr := entry.Info()
		if statErr != nil {
			plan.Warnings = append(plan.Warnings, statErr.Error())
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			plan.Skipped = append(plan.Skipped, path)
			return nil
		}
		rel, relErr := filepath.Rel(run, path)
		if relErr != nil {
			plan.Warnings = append(plan.Warnings, relErr.Error())
			return nil
		}
		plan.Copies = append(plan.Copies, copyPlanEntry{
			Source:      path,
			Destination: filepath.Join(destinationRoot, rel),
			Bytes:       info.Size(),
		})
		plan.FileCount++
		plan.ByteCount += info.Size()
		return nil
	})
	if err != nil {
		return persistPlan{}, err
	}
	return plan, nil
}

func cleanExistingDir(path string, flag string) (string, error) {
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%s invalid: %w", flag, err)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", flag, cleaned, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s must be a directory: %s", flag, cleaned)
	}
	return cleaned, nil
}

func extractUserDecisions(status taskStatusSummary, snippets []sourceSnippet, planContent string) []string {
	var decisions []string
	for _, line := range strings.Split(planContent, "\n") {
		value := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if containsAny(strings.ToLower(value), []string{"decision", "decided", "user asked", "用户", "决定"}) {
			decisions = append(decisions, value)
		}
	}
	for _, blocker := range status.Blockers {
		if containsAny(strings.ToLower(blocker), []string{"user", "ask", "decision", "用户", "决定"}) {
			decisions = append(decisions, blocker)
		}
	}
	for _, snippet := range snippets {
		if containsAny(strings.ToLower(snippet.Text), []string{"decision", "decided", "user", "用户", "决定"}) {
			decisions = append(decisions, snippet.Text)
		}
	}
	return dedupeLimited(decisions, 8)
}

func nextCommands(snapshot interface{}) []string {
	commands := []string{
		"anton context --json",
		"anton task-state check --json",
		"anton gates check --json",
		"anton handoff build --json",
	}
	_ = snapshot
	return commands
}

func isSourceSnippetCandidate(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower, []string{
		"blocker", "blocked", "decision", "decided", "next", "command", "handoff",
		"summary", "failed", "failure", "error", "todo", "api_key", "token", "secret",
		"password", "authorization", "用户", "决定", "下一步",
	})
}

func redactHandoffText(value string) string {
	redacted := value
	for _, pattern := range handoffRedactionPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}

func compact(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 500 {
		return value[:500] + "...[truncated]"
	}
	return value
}

func trimList(values []string) []string {
	var trimmed []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func dedupeLimited(values []string, limit int) []string {
	seen := map[string]bool{}
	var output []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
		if len(output) >= limit {
			break
		}
	}
	return output
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envMap(environ []string) map[string]string {
	values := map[string]string{}
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}
