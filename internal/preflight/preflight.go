package preflight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/gates"
)

const (
	statusOK      = "ok"
	statusWarning = "warning"
	statusBlocked = "blocked"
	statusSkipped = "skipped"

	profileInvestigation  = "investigation"
	profileImplementation = "implementation"
)

type options struct {
	JSON    bool
	Profile string
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

type finding struct {
	Level       string `json:"level"`
	Code        string `json:"code"`
	Surface     string `json:"surface"`
	Path        string `json:"path,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type summary struct {
	Status       string `json:"status"`
	OKCount      int    `json:"ok_count"`
	WarningCount int    `json:"warning_count"`
	BlockedCount int    `json:"blocked_count"`
	SkippedCount int    `json:"skipped_count"`
}

type gitSummary struct {
	RepositoryRoot string `json:"repository_root,omitempty"`
	RepositoryKind string `json:"repository_kind,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Dirty          bool   `json:"dirty"`
	ChangeCount    int    `json:"change_count"`
}

type taskStateData struct {
	Identity   adapter.TaskIdentity `json:"identity"`
	BundleRoot string               `json:"bundle_root,omitempty"`
	StatusPath string               `json:"status_path,omitempty"`
	Lifecycle  string               `json:"lifecycle,omitempty"`
	UpdatedAt  string               `json:"updated_at,omitempty"`
}

type reportData struct {
	SchemaVersion string               `json:"schema_version"`
	Profile       string               `json:"profile"`
	Adapter       string               `json:"adapter,omitempty"`
	Contract      *contract.ContractV1 `json:"contract,omitempty"`
	TaskState     taskStateData        `json:"task_state"`
	Git           gitSummary           `json:"git"`
	Gates         *gates.Set           `json:"gates,omitempty"`
	Checks        []check              `json:"checks"`
	Findings      []finding            `json:"findings,omitempty"`
	Summary       summary              `json:"summary"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *reportData   `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if hasHelp(args) {
		_, _ = io.WriteString(stdout, usageText())
		return 0
	}

	opts, err := parseOptions(args)
	if err != nil {
		return writeError("preflight", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	data, err := collect(opts.Profile, environ)
	if err != nil {
		return writeError("preflight", "preflight-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	output := response{
		OK:      data.Summary.BlockedCount == 0,
		Command: "preflight",
		Data:    &data,
	}
	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(output)
	} else {
		renderHuman(stdout, output)
	}
	if output.OK {
		return 0
	}
	return 1
}

func usageText() string {
	return `Usage:
  anton preflight --profile investigation [--json]
  anton preflight --profile implementation [--json]

Runs read-only start-work checks for coding agents.
`
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--profile":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --profile")
			}
			opts.Profile = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	switch opts.Profile {
	case profileInvestigation, profileImplementation:
		return opts, nil
	case "":
		return opts, fmt.Errorf("missing required --profile investigation|implementation")
	default:
		return opts, fmt.Errorf("unsupported profile %q; expected investigation or implementation", opts.Profile)
	}
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

func collect(profile string, environ []string) (reportData, error) {
	wd, err := os.Getwd()
	if err != nil {
		return reportData{}, fmt.Errorf("get working directory: %w", err)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return blockedConfigReport(profile, wd, environ, err), nil
	}

	contextData := resolved.Context
	entrypointPath := resolved.Definition.EntrypointPath(contextData)
	taskIdentity := adapter.ResolveTaskIdentity(contextData, resolved.Config, environ)
	gitData, gitCheck := inspectGit(contextData)

	checks := []check{
		{
			Name:   "anton-config",
			Status: statusOK,
			Detail: fmt.Sprintf("loaded %s from %s", resolved.Config.Source(), resolved.Config.Path),
		},
		checkEntrypoint(entrypointPath),
	}

	taskState, taskChecks, taskFindings := checkTaskState(profile, resolved, taskIdentity, environ)
	checks = append(checks, taskChecks...)
	if profile == profileImplementation {
		checks = append(checks, checkWritable(contextData.WorkingDirectory))
	} else {
		checks = append(checks, check{Name: "working-directory-writable", Status: statusSkipped, Detail: "investigation profile does not write probe files"})
	}
	checks = append(checks, gitCheck)

	gateSet, gateCheck, gateFindings := checkGates(resolved.Config)
	checks = append(checks, gateCheck, checkOptionalIntegrations())

	findings := append(taskFindings, gateFindings...)
	for _, item := range checks {
		if item.Status == statusBlocked {
			findings = append(findings, findingFromCheck(item, "preflight", "preflight-blocked"))
		}
	}
	summaryData := summarize(checks)
	contractData := contract.Build(contract.Input{
		Adapter:        resolved.Definition.Name(),
		Context:        contextData,
		Config:         resolved.Config,
		EntrypointPath: entrypointPath,
		TaskIdentity:   taskIdentity,
		Checks:         checksToContract(checks),
		Summary: contract.Summary{
			Status:        summaryData.Status,
			OKCount:       summaryData.OKCount,
			DegradedCount: summaryData.WarningCount + summaryData.SkippedCount,
			BlockedCount:  summaryData.BlockedCount,
		},
	})

	return reportData{
		SchemaVersion: "anton.preflight.v1",
		Profile:       profile,
		Adapter:       resolved.Definition.Name(),
		Contract:      &contractData,
		TaskState:     taskState,
		Git:           gitData,
		Gates:         &gateSet,
		Checks:        checks,
		Findings:      findings,
		Summary:       summaryData,
	}, nil
}

func blockedConfigReport(profile string, wd string, environ []string, err error) reportData {
	contextData, _ := adapter.DetectContext(wd, environ)
	gitData, _ := inspectGit(contextData)
	checks := []check{
		{
			Name:   "anton-config",
			Status: statusBlocked,
			Detail: err.Error(),
			Hint:   "fix anton.yaml so Anton can resolve the canonical repo contract",
		},
	}
	return reportData{
		SchemaVersion: "anton.preflight.v1",
		Profile:       profile,
		Git:           gitData,
		Checks:        checks,
		Findings: []finding{
			{
				Level:       statusBlocked,
				Code:        "invalid-config",
				Surface:     "config",
				Message:     err.Error(),
				Remediation: "fix anton.yaml so Anton can resolve the canonical repo contract",
			},
		},
		Summary: summarize(checks),
	}
}

func checkEntrypoint(path string) check {
	info, err := os.Stat(path)
	switch {
	case err == nil && !info.IsDir():
		return check{Name: "entrypoint-file", Status: statusOK, Detail: fmt.Sprintf("found configured entrypoint at %s", path)}
	case err == nil && info.IsDir():
		return check{Name: "entrypoint-file", Status: statusBlocked, Detail: fmt.Sprintf("configured entrypoint is a directory: %s", path), Hint: "set entrypoint.path to an agent entrypoint file"}
	case os.IsNotExist(err):
		return check{Name: "entrypoint-file", Status: statusBlocked, Detail: fmt.Sprintf("configured entrypoint is missing: %s", path), Hint: "create the entrypoint file or update entrypoint.path in anton.yaml"}
	default:
		return check{Name: "entrypoint-file", Status: statusBlocked, Detail: fmt.Sprintf("cannot stat configured entrypoint %s: %v", path, err), Hint: "fix entrypoint file permissions or config path"}
	}
}

func checkTaskState(profile string, resolved adapter.Resolved, identity adapter.TaskIdentity, environ []string) (taskStateData, []check, []finding) {
	data := taskStateData{Identity: identity}
	if identity.Conflict {
		item := check{
			Name:   "task-identity",
			Status: statusBlocked,
			Detail: fmt.Sprintf("conflicting task identity signals detected: %s", strings.Join(identity.ConflictValues, ", ")),
			Hint:   "align ANTON_TASK_ID, task/<id_slug> branch, and current task bundle before continuing",
		}
		return data, []check{item}, []finding{findingFromCheck(item, "task-state", "task-identity-conflict")}
	}
	if strings.TrimSpace(identity.Resolved) == "" {
		status := statusWarning
		if profile == profileImplementation {
			status = statusBlocked
		}
		item := check{
			Name:   "task-identity",
			Status: status,
			Detail: "task identity could not be inferred from env, branch, or bundle path",
			Hint:   "set ANTON_TASK_ID, use a task/<id_slug> branch, or run inside an existing bundle",
		}
		return data, []check{item}, []finding{findingFromCheck(item, "task-state", "task-identity-missing")}
	}

	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, time.Now())
	if err != nil {
		item := check{Name: "task-state", Status: statusBlocked, Detail: err.Error(), Hint: "fix task identity and task root configuration before starting work"}
		return data, []check{item}, []finding{findingFromCheck(item, "task-state", "task-state-unresolved")}
	}
	data.BundleRoot = bundle.Root
	data.StatusPath = bundle.StatusPath()

	snapshot, err := resolved.Definition.ReadStatus(data.StatusPath)
	if err != nil {
		item := check{Name: "task-state", Status: statusBlocked, Detail: fmt.Sprintf("cannot read status.yaml at %s: %v", data.StatusPath, err), Hint: "run anton task-state init --json or fix the existing status.yaml"}
		return data, []check{{Name: "task-identity", Status: statusOK, Detail: fmt.Sprintf("resolved task id %s", identity.Resolved)}, item}, []finding{findingFromCheck(item, "task-state", "task-state-unreadable")}
	}
	data.Lifecycle = snapshot.Lifecycle
	data.UpdatedAt = snapshot.UpdatedAt
	if snapshot.TaskID != identity.Resolved {
		item := check{Name: "task-state", Status: statusBlocked, Detail: fmt.Sprintf("status task id %q differs from resolved task id %q", snapshot.TaskID, identity.Resolved), Hint: "retarget the task bundle or align ANTON_TASK_ID with status.yaml"}
		return data, []check{{Name: "task-identity", Status: statusOK, Detail: fmt.Sprintf("resolved task id %s", identity.Resolved)}, item}, []finding{findingFromCheck(item, "task-state", "task-state-identity-mismatch")}
	}

	return data, []check{
		{Name: "task-identity", Status: statusOK, Detail: fmt.Sprintf("resolved task id %s", identity.Resolved)},
		{Name: "task-state", Status: statusOK, Detail: fmt.Sprintf("read status.yaml at %s", data.StatusPath)},
	}, nil
}

func checkWritable(wd string) check {
	file, err := os.CreateTemp(wd, ".anton-preflight-*")
	if err != nil {
		return check{Name: "working-directory-writable", Status: statusBlocked, Detail: fmt.Sprintf("cannot create temp file in %s: %v", wd, err), Hint: "fix directory permissions or run Anton from a writable path"}
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	switch {
	case closeErr != nil:
		_ = os.Remove(name)
		return check{Name: "working-directory-writable", Status: statusBlocked, Detail: fmt.Sprintf("created temp file but close failed in %s: %v", wd, closeErr), Hint: "inspect filesystem health before starting work"}
	case removeErr != nil:
		return check{Name: "working-directory-writable", Status: statusBlocked, Detail: fmt.Sprintf("created temp file but cleanup failed at %s: %v", name, removeErr), Hint: "remove the temp file and inspect directory permissions"}
	default:
		return check{Name: "working-directory-writable", Status: statusOK, Detail: fmt.Sprintf("created and removed a temp file in %s", wd)}
	}
}

func inspectGit(contextData adapter.Context) (gitSummary, check) {
	data := gitSummary{
		RepositoryRoot: contextData.RepositoryRoot,
		RepositoryKind: contextData.RepositoryKind,
		Branch:         contextData.GitBranch,
	}
	if contextData.RepositoryRoot == "" {
		return data, check{Name: "git-state", Status: statusSkipped, Detail: "working directory is not inside a git repository"}
	}

	cmd := exec.Command("git", "-C", contextData.RepositoryRoot, "status", "--short")
	output, err := cmd.Output()
	if err != nil {
		return data, check{Name: "git-state", Status: statusWarning, Detail: fmt.Sprintf("git status summary unavailable: %v", err), Hint: "inspect git status manually before editing"}
	}
	data.ChangeCount = countNonEmptyLines(output)
	data.Dirty = data.ChangeCount > 0
	return data, check{Name: "git-state", Status: statusOK, Detail: fmt.Sprintf("git repository %s on %s with %d visible change(s)", contextData.RepositoryRoot, contextData.GitBranch, data.ChangeCount)}
}

func checkGates(config adapter.Config) (gates.Set, check, []finding) {
	var set gates.Set
	var err error
	if config.Loaded {
		set, err = gates.LoadFile(config.Path, config.Source())
	} else {
		set = gates.EmptySet(config.Path)
	}
	if err != nil {
		item := check{Name: "gates", Status: statusBlocked, Detail: err.Error(), Hint: "fix declarative gates metadata in anton.yaml"}
		return set, item, []finding{findingFromCheck(item, "gates", "gates-invalid")}
	}

	switch {
	case set.Summary.Errors > 0:
		item := check{Name: "gates", Status: statusBlocked, Detail: fmt.Sprintf("gate metadata has %d error(s)", set.Summary.Errors), Hint: "fix declarative gates metadata before starting implementation"}
		return set, item, []finding{findingFromCheck(item, "gates", "gates-invalid")}
	case set.Summary.Warnings > 0:
		return set, check{Name: "gates", Status: statusWarning, Detail: fmt.Sprintf("gate metadata has %d warning(s)", set.Summary.Warnings), Hint: "review gate metadata; preflight does not execute gates"}, nil
	default:
		return set, check{Name: "gates", Status: statusOK, Detail: fmt.Sprintf("gate metadata loaded with %d declared gate(s)", set.Summary.Declared)}, nil
	}
}

func checkOptionalIntegrations() check {
	return check{
		Name:   "optional-integrations",
		Status: statusSkipped,
		Detail: "no optional preflight integrations are configured",
	}
}

func summarize(checks []check) summary {
	result := summary{Status: statusOK}
	for _, item := range checks {
		switch item.Status {
		case statusOK:
			result.OKCount++
		case statusWarning:
			result.WarningCount++
		case statusBlocked:
			result.BlockedCount++
		case statusSkipped:
			result.SkippedCount++
		}
	}
	if result.BlockedCount > 0 {
		result.Status = statusBlocked
	} else if result.WarningCount > 0 {
		result.Status = statusWarning
	}
	return result
}

func checksToContract(checks []check) []contract.Check {
	result := make([]contract.Check, 0, len(checks))
	for _, item := range checks {
		result = append(result, contract.Check{
			Name:   item.Name,
			Status: item.Status,
			Detail: item.Detail,
			Hint:   item.Hint,
		})
	}
	return result
}

func findingFromCheck(item check, surface string, code string) finding {
	return finding{
		Level:       item.Status,
		Code:        code,
		Surface:     surface,
		Message:     item.Detail,
		Remediation: item.Hint,
	}
}

func countNonEmptyLines(output []byte) int {
	count := 0
	for _, line := range bytes.Split(output, []byte("\n")) {
		if strings.TrimSpace(string(line)) != "" {
			count++
		}
	}
	return count
}

func renderHuman(stdout io.Writer, output response) {
	if output.Data == nil {
		return
	}
	data := output.Data
	_, _ = fmt.Fprintf(stdout, "Anton preflight\n")
	_, _ = fmt.Fprintf(stdout, "Profile: %s\n", data.Profile)
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", data.Summary.Status)
	if data.Git.RepositoryRoot != "" {
		_, _ = fmt.Fprintf(stdout, "Repo: %s\n", data.Git.RepositoryRoot)
	}
	for _, item := range data.Checks {
		_, _ = fmt.Fprintf(stdout, "- %s: %s", item.Name, item.Status)
		if item.Detail != "" {
			_, _ = fmt.Fprintf(stdout, " - %s", item.Detail)
		}
		_, _ = fmt.Fprintln(stdout)
	}
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
