package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WorktreeStatus holds information about a single git worktree.
type WorktreeStatus struct {
	Path              string `json:"path"`
	Branch            string `json:"branch"`
	Head              string `json:"head"`
	IsBare            bool   `json:"is_bare"`
	LastCommitAgeDays int    `json:"last_commit_age_days"`
	HasUncommitted    bool   `json:"has_uncommitted"`
	HasArtifacts      bool   `json:"has_artifacts"`
	SafeToRemove      bool   `json:"safe_to_remove"`
	Blocker           string `json:"blocker,omitempty"`
}

// artifactDirs are subdirectory names that indicate artifact presence.
var artifactDirs = []string{"results", "outputs", "experiments", "data"}

// worktreeEntry is the raw parsed data from a single `git worktree list --porcelain` block.
type worktreeEntry struct {
	path   string
	head   string
	branch string
	bare   bool
}

// parseWorktreePorcelain parses the output of `git worktree list --porcelain`
// and returns all worktree entries (main worktree first).
// Each block is separated by a blank line.
func parseWorktreePorcelain(output string) []worktreeEntry {
	var entries []worktreeEntry
	var current worktreeEntry
	inBlock := false

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" {
			if inBlock && current.path != "" {
				entries = append(entries, current)
				current = worktreeEntry{}
				inBlock = false
			}
			continue
		}
		inBlock = true
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.path = filepath.Clean(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "HEAD "):
			current.head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			current.bare = true
		}
	}
	// Flush last block if output doesn't end with a blank line.
	if inBlock && current.path != "" {
		entries = append(entries, current)
	}
	return entries
}

// listWorktrees returns WorktreeStatus for every non-main worktree under repoRoot.
func listWorktrees(repoRoot string) ([]WorktreeStatus, error) {
	output, err := gitWorktreeCommand(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	raw := parseWorktreePorcelain(output)
	if len(raw) == 0 {
		return []WorktreeStatus{}, nil
	}

	// The first entry is the main worktree — skip it.
	var result []WorktreeStatus
	for _, e := range raw[1:] {
		status, err := buildWorktreeStatus(e.path, e.head, e.branch, e.bare)
		if err != nil {
			// Best-effort: include a partially-populated entry rather than failing.
			status = WorktreeStatus{
				Path:   e.path,
				Branch: e.branch,
				Head:   e.head,
				IsBare: e.bare,
			}
		}
		result = append(result, status)
	}
	return result, nil
}

// inspectWorktree returns a WorktreeStatus for a single worktree path.
func inspectWorktree(path string) (WorktreeStatus, error) {
	path = filepath.Clean(path)

	headOut, err := gitWorktreeCommand(path, "log", "-1", "--format=%H")
	head := strings.TrimSpace(headOut)
	if err != nil || head == "" {
		head = ""
	}

	branchOut, _ := gitWorktreeCommand(path, "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(branchOut)

	return buildWorktreeStatus(path, head, branch, false)
}

// buildWorktreeStatus populates a WorktreeStatus from path and pre-fetched git data.
func buildWorktreeStatus(path string, head string, branch string, bare bool) (WorktreeStatus, error) {
	status := WorktreeStatus{
		Path:   path,
		Branch: branch,
		Head:   head,
		IsBare: bare,
	}

	// Last commit age.
	tsOut, err := gitWorktreeCommand(path, "log", "-1", "--format=%ct")
	if err == nil {
		ts := strings.TrimSpace(tsOut)
		if unix, err2 := strconv.ParseInt(ts, 10, 64); err2 == nil {
			age := time.Since(time.Unix(unix, 0))
			status.LastCommitAgeDays = int(age.Hours() / 24)
		}
	}

	// Uncommitted changes.
	statusOut, ok := gitCommandStatus(path, "status", "--short")
	if ok && strings.TrimSpace(statusOut) != "" {
		status.HasUncommitted = true
	}

	// Artifacts.
	for _, dir := range artifactDirs {
		candidate := filepath.Join(path, dir)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			status.HasArtifacts = true
			break
		}
	}

	// Safety.
	status.SafeToRemove = !status.HasUncommitted && !status.HasArtifacts
	if !status.SafeToRemove {
		var blockers []string
		if status.HasUncommitted {
			blockers = append(blockers, "uncommitted changes")
		}
		if status.HasArtifacts {
			blockers = append(blockers, "artifact directories present")
		}
		status.Blocker = strings.Join(blockers, "; ")
	}

	return status, nil
}

// removeWorktree removes a worktree, optionally forcing.
func removeWorktree(path string, dryRun bool, force bool, repoRoot string) error {
	if dryRun {
		return nil
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := gitWorktreeCommand(repoRoot, args...)
	return err
}

// gitWorktreeCommand runs a git command and returns (output, error).
func gitWorktreeCommand(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// worktreesResponse is the JSON envelope for worktrees subcommands.
type worktreesResponse struct {
	OK      bool             `json:"ok"`
	Command string           `json:"command"`
	Data    []WorktreeStatus `json:"data,omitempty"`
	Single  *WorktreeStatus  `json:"worktree,omitempty"`
	Error   *errorPayload    `json:"error,omitempty"`
}

// runWorktrees is the entry point for `anton workspace worktrees <subcommand>`.
func runWorktrees(args []string, command string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: %s list|inspect <path>|remove [--dry-run] [--force] <path>\n", command)
		return 2
	}

	subcommand := args[0]
	rest := args[1:]

	switch subcommand {
	case "list":
		return runWorktreesList(rest, command+" list", stdout, stderr, environ)
	case "inspect":
		return runWorktreesInspect(rest, command+" inspect", stdout, stderr)
	case "remove":
		return runWorktreesRemove(rest, command+" remove", stdout, stderr, environ)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown worktrees subcommand: %s\n", subcommand)
		return 2
	}
}

func runWorktreesList(args []string, command string, stdout io.Writer, stderr io.Writer, environ []string) int {
	repoRoot, err := resolveRepoRoot(environ)
	if err != nil {
		return writeWorktreesError(command, "repo-root-failed", err.Error(), stdout, stderr)
	}

	list, err := listWorktrees(repoRoot)
	if err != nil {
		return writeWorktreesError(command, "worktrees-list-failed", err.Error(), stdout, stderr)
	}

	payload := worktreesResponse{OK: true, Command: command, Data: list}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
	return 0
}

func runWorktreesInspect(args []string, command string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: %s <path>\n", command)
		return 2
	}
	path := args[0]
	status, err := inspectWorktree(path)
	if err != nil {
		return writeWorktreesError(command, "worktrees-inspect-failed", err.Error(), stdout, stderr)
	}
	payload := worktreesResponse{OK: true, Command: command, Single: &status}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
	return 0
}

func runWorktreesRemove(args []string, command string, stdout io.Writer, stderr io.Writer, environ []string) int {
	dryRun := false
	force := false
	var remaining []string
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		default:
			remaining = append(remaining, arg)
		}
	}
	if len(remaining) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: %s [--dry-run] [--force] <path>\n", command)
		return 2
	}
	path := remaining[0]

	repoRoot, err := resolveRepoRoot(environ)
	if err != nil {
		return writeWorktreesError(command, "repo-root-failed", err.Error(), stdout, stderr)
	}

	if dryRun {
		status, _ := inspectWorktree(path)
		payload := worktreesResponse{OK: true, Command: command, Single: &status}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return 0
	}

	if err := removeWorktree(path, dryRun, force, repoRoot); err != nil {
		return writeWorktreesError(command, "worktrees-remove-failed", err.Error(), stdout, stderr)
	}

	payload := worktreesResponse{OK: true, Command: command}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
	return 0
}

func writeWorktreesError(command string, code string, message string, stdout io.Writer, stderr io.Writer) int {
	payload := worktreesResponse{OK: false, Command: command, Error: &errorPayload{Code: code, Message: message}}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return 1
}

// resolveRepoRoot finds the repository root via git rev-parse, falling back to cwd.
func resolveRepoRoot(environ []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", "-C", wd, "rev-parse", "--show-toplevel")
	env := os.Environ()
	env = append(env, environ...)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return wd, nil // fall back to cwd
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}
