package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

type options struct {
	JSON   bool
	Target string
}

type project struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type rootStatus struct {
	Configured string    `json:"configured"`
	Absolute   string    `json:"absolute"`
	Exists     bool      `json:"exists"`
	Status     string    `json:"status"`
	Projects   []project `json:"projects"`
	Findings   []finding `json:"findings,omitempty"`
}

type finding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type summary struct {
	Status       string `json:"status"`
	RootCount    int    `json:"root_count"`
	ProjectCount int    `json:"project_count"`
	WarningCount int    `json:"warning_count"`
	ErrorCount   int    `json:"error_count"`
}

type commandData struct {
	Adapter          string       `json:"adapter"`
	WorkingDirectory string       `json:"working_directory"`
	RepositoryRoot   string       `json:"repository_root,omitempty"`
	ConfigPath       string       `json:"config_path"`
	ConfigSource     string       `json:"config_source"`
	Roots            []rootStatus `json:"roots"`
	Findings         []finding    `json:"findings"`
	Summary          summary      `json:"summary"`
	Refs             *RefsReport  `json:"refs,omitempty"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *commandData  `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}
	switch args[0] {
	case "inspect":
		return run(args[1:], "workspace inspect", stdout, stderr, environ)
	case "check":
		return run(args[1:], "workspace check", stdout, stderr, environ)
	case "refs":
		return runRefs(args[1:], stdout, stderr, environ)
	case "prepare":
		opts, err := parseOptions(args[1:])
		if err != nil {
			return writeError("workspace prepare", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
		}
		return writeError("workspace prepare", "not-approved", "workspace prepare is not approved until a separate write-safety plan lands", opts.JSON, stdout, stderr, 2)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown workspace command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runRefs(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("workspace refs", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	report, err := BuildRefsReport(environ, opts.Target)
	if err != nil {
		return writeError("workspace refs", "workspace-refs-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	data := commandData{
		Adapter:          report.Adapter,
		WorkingDirectory: report.WorkingDirectory,
		RepositoryRoot:   report.RepositoryRoot,
		ConfigPath:       report.ConfigPath,
		ConfigSource:     report.ConfigSource,
		Findings:         refsFindings(report.Findings),
		Summary: summary{
			Status:       report.Summary.Status,
			WarningCount: report.Summary.Warnings,
			ErrorCount:   report.Summary.Blockers,
		},
		Refs: &report,
	}
	exitCode := 0
	if report.Summary.Blockers > 0 {
		exitCode = 1
	}
	return writeResponse("workspace refs", data, opts.JSON, stdout, exitCode)
}

func run(args []string, command string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError(command, "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	data, err := inspect(environ)
	if err != nil {
		return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	exitCode := 0
	if command == "workspace check" && data.Summary.ErrorCount > 0 {
		exitCode = 1
	}
	return writeResponse(command, data, opts.JSON, stdout, exitCode)
}

func inspect(environ []string) (commandData, error) {
	wd, err := os.Getwd()
	if err != nil {
		return commandData{}, err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return commandData{}, err
	}
	base := resolved.Context.WorkingDirectory
	if resolved.Context.RepositoryRoot != "" {
		base = resolved.Context.RepositoryRoot
	}
	roots := []rootStatus{}
	for _, configured := range resolved.Config.Threads.WorkspaceRoots {
		roots = append(roots, inspectRoot(base, configured))
	}
	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		RepositoryRoot:   resolved.Context.RepositoryRoot,
		ConfigPath:       resolved.Config.Path,
		ConfigSource:     resolved.Config.Source(),
		Roots:            roots,
		Findings:         flattenFindings(roots),
	}
	data.Summary = summarize(roots)
	return data, nil
}

func inspectRoot(base string, configured string) rootStatus {
	absolute := configured
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(base, configured)
	}
	absolute = filepath.Clean(absolute)
	status := rootStatus{Configured: configured, Absolute: absolute, Status: "ok", Projects: []project{}}
	if strings.Contains(configured, "..") {
		status.Status = "blocked"
		status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-root-traversal", Path: configured, Message: "workspace root must not contain traversal segments"})
		return status
	}
	if !pathWithinRoot(base, absolute) {
		status.Status = "blocked"
		status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-root-escapes-repo", Path: absolute, Message: "workspace root escapes the repository boundary"})
		return status
	}
	if real, err := filepath.EvalSymlinks(absolute); err == nil && !pathWithinRoot(base, real) {
		status.Status = "blocked"
		status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-root-symlink-escape", Path: absolute, Message: "workspace root resolves outside the repository boundary"})
		return status
	}
	info, err := os.Stat(absolute)
	switch {
	case err == nil && info.IsDir():
		status.Exists = true
		status.Projects = listProjects(base, absolute)
		for _, item := range status.Projects {
			if real, err := filepath.EvalSymlinks(item.Path); err == nil && !pathWithinRoot(base, real) {
				status.Status = "blocked"
				status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-project-symlink-escape", Path: item.Path, Message: "workspace project resolves outside the repository boundary"})
			}
		}
	case err == nil && !info.IsDir():
		status.Status = "blocked"
		status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-root-not-directory", Path: absolute, Message: "workspace root exists but is not a directory"})
	case os.IsNotExist(err):
		status.Status = "warning"
		status.Findings = append(status.Findings, finding{Level: "warning", Code: "workspace-root-missing", Path: absolute, Message: "workspace root does not exist; inspect/check will not create it"})
	default:
		status.Status = "blocked"
		status.Findings = append(status.Findings, finding{Level: "error", Code: "workspace-root-stat-failed", Path: absolute, Message: err.Error()})
	}
	return status
}

func listProjects(base string, root string) []project {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []project{}
	}
	projects := []project{}
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !pathWithinRoot(root, path) || !pathWithinRoot(base, path) {
			continue
		}
		projects = append(projects, project{Name: entry.Name(), Path: path})
	}
	return projects
}

func flattenFindings(roots []rootStatus) []finding {
	findings := []finding{}
	for _, root := range roots {
		findings = append(findings, root.Findings...)
	}
	return findings
}

func summarize(roots []rootStatus) summary {
	result := summary{Status: "ok", RootCount: len(roots)}
	for _, root := range roots {
		result.ProjectCount += len(root.Projects)
		for _, finding := range root.Findings {
			switch finding.Level {
			case "error":
				result.ErrorCount++
				result.Status = "blocked"
			case "warning":
				result.WarningCount++
				if result.Status != "blocked" {
					result.Status = "degraded"
				}
			}
		}
	}
	return result
}

func pathWithinRoot(root string, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if root == candidate {
		return true
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--target":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --target")
			}
			opts.Target = args[index]
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton workspace inspect [--json]
  anton workspace check [--json]
  anton workspace refs --target PATH [--json]
`
}

func writeResponse(command string, data commandData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{OK: exitCode == 0, Command: command, Data: &data}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stdout, "Anton %s\nStatus: %s\nRoots: %d\n", command, data.Summary.Status, data.Summary.RootCount)
	if data.Refs != nil {
		_, _ = fmt.Fprintf(stdout, "Target: %s\nReferences: %d\nRecommendation: %s\n", data.Refs.Target.Relative, len(data.Refs.ReferenceHits), data.Refs.Summary.Recommendation)
	}
	return exitCode
}

func refsFindings(refFindings []RefFinding) []finding {
	findings := make([]finding, 0, len(refFindings))
	for _, item := range refFindings {
		findings = append(findings, finding{
			Level:   item.Level,
			Code:    item.Code,
			Path:    item.Path,
			Message: item.Message,
		})
	}
	return findings
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := response{OK: false, Command: command, Error: &errorPayload{Code: code, Message: message}}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
