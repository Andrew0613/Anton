package adopt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/doctor"
)

const (
	severityInfo    = "info"
	severityWarning = "warning"
	severityBlocked = "blocked"
)

type Gap struct {
	Code        string   `json:"code"`
	Module      string   `json:"module"`
	Severity    string   `json:"severity"`
	Source      string   `json:"source"`
	Confidence  string   `json:"confidence"`
	Path        string   `json:"path,omitempty"`
	Detail      string   `json:"detail"`
	Remediation []string `json:"remediation"`
}

type Summary struct {
	Status       string `json:"status"`
	GapCount     int    `json:"gap_count"`
	InfoCount    int    `json:"info_count"`
	WarningCount int    `json:"warning_count"`
	BlockedCount int    `json:"blocked_count"`
}

type ConfigSnapshot struct {
	Source         string   `json:"source"`
	Path           string   `json:"path"`
	EntrypointPath string   `json:"entrypoint_path"`
	TasksRoot      string   `json:"tasks_root"`
	TasksRootPath  string   `json:"tasks_root_path"`
	WorkspaceRoots []string `json:"workspace_roots,omitempty"`
}

type Plan struct {
	Adapter          string         `json:"adapter"`
	WorkingDirectory string         `json:"working_directory"`
	WorkspaceKind    string         `json:"workspace_kind"`
	RepositoryRoot   string         `json:"repository_root,omitempty"`
	Config           ConfigSnapshot `json:"config"`
	Gaps             []Gap          `json:"gaps"`
	Summary          Summary        `json:"summary"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *Plan         `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type options struct {
	JSON  bool
	Human bool
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown adopt command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runPlan(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("adopt plan", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	contractData, err := doctor.CollectContract(environ)
	if err != nil {
		return writeError("adopt plan", "contract", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	plan := Analyze(contractData)
	output := response{
		OK:      true,
		Command: "adopt plan",
		Data:    &plan,
	}
	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(output)
		return 0
	}

	renderHuman(stdout, output)
	return 0
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.JSON = true
		case "--human":
			opts.Human = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if opts.JSON && opts.Human {
		return opts, fmt.Errorf("--json and --human cannot be combined")
	}
	return opts, nil
}

func Analyze(contractData contract.ContractV1) Plan {
	base := contractData.Context.RepositoryRoot
	if strings.TrimSpace(base) == "" {
		base = contractData.Context.WorkingDirectory
	}

	tasksRootPath := resolvePath(base, contractData.Config.TasksRoot)
	plan := Plan{
		Adapter:          contractData.Adapter,
		WorkingDirectory: contractData.Context.WorkingDirectory,
		WorkspaceKind:    contractData.Context.WorkspaceKind,
		RepositoryRoot:   contractData.Context.RepositoryRoot,
		Gaps:             []Gap{},
		Config: ConfigSnapshot{
			Source:         contractData.Config.Source,
			Path:           contractData.Config.Path,
			EntrypointPath: contractData.Config.EntrypointPath,
			TasksRoot:      contractData.Config.TasksRoot,
			TasksRootPath:  tasksRootPath,
			WorkspaceRoots: contractData.Config.ThreadsWorkspaceRoots,
		},
	}

	plan.Gaps = append(plan.Gaps, configGaps(contractData)...)
	plan.Gaps = append(plan.Gaps, entrypointGaps(base, contractData.Config.EntrypointPath)...)
	plan.Gaps = append(plan.Gaps, taskLayoutGaps(base, tasksRootPath)...)
	plan.Gaps = append(plan.Gaps, threadsWorkspaceGaps(base, contractData.Config.ThreadsWorkspaceRoots)...)
	plan.Gaps = append(plan.Gaps, contractHealthGaps(contractData)...)
	plan.Summary = summarize(plan.Gaps)

	return plan
}

func configGaps(contractData contract.ContractV1) []Gap {
	switch contractData.Config.Source {
	case "built-in defaults":
		return []Gap{{
			Code:       "config-missing",
			Module:     "config",
			Severity:   severityWarning,
			Source:     "contract.config.source",
			Confidence: "high",
			Path:       contractData.Config.Path,
			Detail:     "repo is using built-in Anton defaults instead of an explicit anton.yaml",
			Remediation: []string{
				"add a repo-local anton.yaml that declares entrypoint.path, tasks.root, and threads defaults",
			},
		}}
	case "inherited main-checkout anton.yaml":
		return []Gap{{
			Code:       "config-inherited",
			Module:     "config",
			Severity:   severityInfo,
			Source:     "contract.config.source",
			Confidence: "high",
			Path:       contractData.Config.Path,
			Detail:     "this worktree inherits anton.yaml from the main checkout",
			Remediation: []string{
				"keep the inherited config intentional, or add a worktree-local anton.yaml if this checkout needs a different contract",
			},
		}}
	default:
		return nil
	}
}

func entrypointGaps(base string, entrypointPath string) []Gap {
	if outsideBoundary(base, entrypointPath) {
		return []Gap{{
			Code:       "entrypoint-outside-repo",
			Module:     "entrypoint",
			Severity:   severityBlocked,
			Source:     "contract.config.entrypoint_path",
			Confidence: "high",
			Path:       entrypointPath,
			Detail:     "configured entrypoint is outside the resolved repo boundary, so adopt plan did not inspect it",
			Remediation: []string{
				"point entrypoint.path at a repo-local file such as AGENTS.md",
			},
		}}
	}

	if _, err := os.Stat(entrypointPath); err == nil {
		return nil
	} else if os.IsNotExist(err) {
		return []Gap{{
			Code:       "entrypoint-missing",
			Module:     "entrypoint",
			Severity:   severityWarning,
			Source:     "contract.config.entrypoint_path",
			Confidence: "high",
			Path:       entrypointPath,
			Detail:     "configured entrypoint file is missing",
			Remediation: []string{
				"create the configured entrypoint file and keep it short, with detailed workflow docs linked from docs/",
			},
		}}
	} else {
		return []Gap{{
			Code:       "entrypoint-unreadable",
			Module:     "entrypoint",
			Severity:   severityBlocked,
			Source:     "filesystem.stat",
			Confidence: "high",
			Path:       entrypointPath,
			Detail:     fmt.Sprintf("configured entrypoint could not be inspected: %v", err),
			Remediation: []string{
				"fix permissions or path validity before relying on the entrypoint contract",
			},
		}}
	}
}

func taskLayoutGaps(base string, tasksRootPath string) []Gap {
	if outsideBoundary(base, tasksRootPath) {
		return []Gap{{
			Code:       "tasks-root-outside-repo",
			Module:     "task-state",
			Severity:   severityBlocked,
			Source:     "contract.config.tasks_root",
			Confidence: "high",
			Path:       tasksRootPath,
			Detail:     "configured task root is outside the resolved repo boundary, so adopt plan did not inspect it",
			Remediation: []string{
				"point tasks.root at a repo-local path such as .anton/tasks",
			},
		}}
	}

	info, err := os.Stat(tasksRootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Gap{{
				Code:       "tasks-root-missing",
				Module:     "task-state",
				Severity:   severityWarning,
				Source:     "contract.config.tasks_root",
				Confidence: "high",
				Path:       tasksRootPath,
				Detail:     "configured task root does not exist",
				Remediation: []string{
					"run anton task-state init for the first task or create the canonical .anton/tasks root before adopting task-state workflows",
				},
			}}
		}
		return []Gap{{
			Code:       "tasks-root-unreadable",
			Module:     "task-state",
			Severity:   severityBlocked,
			Source:     "filesystem.stat",
			Confidence: "high",
			Path:       tasksRootPath,
			Detail:     fmt.Sprintf("configured task root could not be inspected: %v", err),
			Remediation: []string{
				"fix permissions or path validity before relying on task-state workflows",
			},
		}}
	}
	if !info.IsDir() {
		return []Gap{{
			Code:       "tasks-root-not-directory",
			Module:     "task-state",
			Severity:   severityBlocked,
			Source:     "filesystem.stat",
			Confidence: "high",
			Path:       tasksRootPath,
			Detail:     "configured task root exists but is not a directory",
			Remediation: []string{
				"replace the file with a canonical task root directory",
			},
		}}
	}

	activePath := filepath.Join(tasksRootPath, "active")
	if activeInfo, activeErr := os.Stat(activePath); activeErr == nil && activeInfo.IsDir() {
		return nil
	}
	return []Gap{{
		Code:       "tasks-active-missing",
		Module:     "task-state",
		Severity:   severityInfo,
		Source:     "canonical-task-layout",
		Confidence: "medium",
		Path:       activePath,
		Detail:     "task root exists but the canonical active/ directory is not present",
		Remediation: []string{
			"create task bundles under .anton/tasks/active/<id_slug>/ with anton task-state init",
		},
	}}
}

func threadsWorkspaceGaps(base string, roots []string) []Gap {
	gaps := []Gap{}
	for _, root := range roots {
		path := resolvePath(base, root)
		if outsideBoundary(base, path) {
			gaps = append(gaps, Gap{
				Code:       "threads-workspace-root-outside-repo",
				Module:     "threads",
				Severity:   severityWarning,
				Source:     "contract.config.threads_workspace_roots",
				Confidence: "high",
				Path:       path,
				Detail:     "declared threads workspace root is outside the resolved repo boundary",
				Remediation: []string{
					"keep threads.workspace_roots repo-local unless the external path is an intentional shared workspace",
				},
			})
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			gaps = append(gaps, Gap{
				Code:       "threads-workspace-root-missing",
				Module:     "threads",
				Severity:   severityInfo,
				Source:     "contract.config.threads_workspace_roots",
				Confidence: "high",
				Path:       path,
				Detail:     "declared threads workspace root does not exist yet",
				Remediation: []string{
					"create the workspace root before relying on workspace-scoped thread discovery",
				},
			})
		}
	}
	return gaps
}

func contractHealthGaps(contractData contract.ContractV1) []Gap {
	gaps := []Gap{}
	seen := map[string]bool{}
	for _, finding := range contractData.Findings {
		if finding.Code == "missing-reference" && filepath.Clean(finding.File) == filepath.Clean(contractData.Config.EntrypointPath) {
			continue
		}
		key := finding.Code + "\x00" + finding.File + "\x00" + finding.Message
		if seen[key] {
			continue
		}
		seen[key] = true

		severity := severityWarning
		if finding.Level == "error" {
			severity = severityBlocked
		}
		gaps = append(gaps, Gap{
			Code:       "contract-" + finding.Code,
			Module:     "contract",
			Severity:   severity,
			Source:     "contract.findings",
			Confidence: "medium",
			Path:       finding.File,
			Detail:     finding.Message,
			Remediation: []string{
				"resolve the reported contract drift or keep it documented as an intentional adoption exception",
			},
		})
	}
	return gaps
}

func summarize(gaps []Gap) Summary {
	result := Summary{Status: "ok", GapCount: len(gaps)}
	for _, gap := range gaps {
		switch gap.Severity {
		case severityInfo:
			result.InfoCount++
		case severityWarning:
			result.WarningCount++
			if result.Status != severityBlocked {
				result.Status = "advisory"
			}
		case severityBlocked:
			result.BlockedCount++
			result.Status = severityBlocked
		}
	}
	return result
}

func resolvePath(base string, pathValue string) string {
	if filepath.IsAbs(pathValue) {
		return filepath.Clean(pathValue)
	}
	return filepath.Clean(filepath.Join(base, pathValue))
}

func outsideBoundary(base string, path string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(path) == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func renderHuman(stdout io.Writer, output response) {
	if output.Data == nil {
		return
	}
	data := output.Data
	_, _ = fmt.Fprintf(stdout, "Anton adopt plan\n")
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", data.Summary.Status)
	_, _ = fmt.Fprintf(stdout, "Repo: %s\n", fallback(data.RepositoryRoot, data.WorkingDirectory))
	_, _ = fmt.Fprintf(stdout, "Config: %s (%s)\n", data.Config.Source, data.Config.Path)
	_, _ = fmt.Fprintf(stdout, "Entrypoint: %s\n", data.Config.EntrypointPath)
	_, _ = fmt.Fprintf(stdout, "Tasks root: %s\n\n", data.Config.TasksRootPath)

	if len(data.Gaps) == 0 {
		_, _ = fmt.Fprintf(stdout, "No adoption gaps found.\n")
		return
	}

	_, _ = fmt.Fprintf(stdout, "Gaps\n")
	for _, gap := range data.Gaps {
		_, _ = fmt.Fprintf(stdout, "  [%s] %s/%s: %s\n", strings.ToUpper(gap.Severity), gap.Module, gap.Code, gap.Detail)
		if gap.Path != "" {
			_, _ = fmt.Fprintf(stdout, "    path: %s\n", gap.Path)
		}
		for _, action := range gap.Remediation {
			_, _ = fmt.Fprintf(stdout, "    - %s\n", action)
		}
	}
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton adopt plan [--json|--human]

Commands:
  plan    report read-only Anton adoption gaps for this repo
`
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := response{
		OK:      false,
		Command: command,
		Error: &errorPayload{
			Code:    code,
			Message: message,
		},
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallbackValue
}
