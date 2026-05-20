package gates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
	runstate "github.com/Andrew0613/Anton/internal/run"
)

const (
	defaultRunTimeout = 300 * time.Second
	outputCapBytes    = 64 * 1024
)

type gateRunResponse struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Receipt *RunReceipt   `json:"receipt,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type RunReceipt struct {
	SchemaVersion int              `json:"schema_version"`
	Kind          string           `json:"kind"`
	Source        Source           `json:"source"`
	RepoRoot      string           `json:"repo_root"`
	Gate          string           `json:"gate,omitempty"`
	Profile       string           `json:"profile,omitempty"`
	DryRun        bool             `json:"dry_run"`
	StartedAt     string           `json:"started_at"`
	EndedAt       string           `json:"ended_at"`
	ReceiptPath   string           `json:"receipt_path,omitempty"`
	AttachRun     *AttachRunMarker `json:"attach_run,omitempty"`
	Summary       RunSummary       `json:"summary"`
	Results       []RunResult      `json:"results"`
}

type AttachRunMarker struct {
	Requested bool   `json:"requested"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type RunSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
	Blocked  int `json:"blocked"`
	TimedOut int `json:"timed_out"`
	Errors   int `json:"errors"`
}

type RunResult struct {
	Gate             string        `json:"gate"`
	Status           string        `json:"status"`
	Reason           string        `json:"reason,omitempty"`
	Type             string        `json:"type"`
	Argv             []string      `json:"argv,omitempty"`
	WorkingDirectory string        `json:"working_directory,omitempty"`
	TimeoutSeconds   int           `json:"timeout_seconds"`
	Destructive      bool          `json:"destructive,omitempty"`
	ExitCode         *int          `json:"exit_code,omitempty"`
	Stdout           OutputSnippet `json:"stdout,omitempty"`
	Stderr           OutputSnippet `json:"stderr,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type OutputSnippet struct {
	Text      string `json:"text,omitempty"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated"`
}

func runDeclaredGates(opts runOptions, stdout io.Writer, stderr io.Writer, environ []string) int {
	set, repoRoot, err := loadRunnerSet(opts, environ)
	if err != nil {
		return writeError("gates run", "gates-run-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	if !set.OK() {
		return writeError("gates run", "invalid-gates-config", "gates config has validation errors; run anton gates check for details", opts.JSON, stdout, stderr, 1)
	}

	selected, err := selectRunnableGates(set, opts)
	if err != nil {
		return writeError("gates run", "gates-run-selection-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	started := time.Now().UTC()
	receipt := RunReceipt{
		SchemaVersion: 1,
		Kind:          "gate-run",
		Source:        set.Source,
		RepoRoot:      filepath.ToSlash(repoRoot),
		Gate:          opts.GateName,
		Profile:       opts.Profile,
		DryRun:        opts.DryRun,
		StartedAt:     started.Format(time.RFC3339Nano),
		Results:       make([]RunResult, 0, len(selected)),
	}
	if opts.AttachRun {
		receipt.AttachRun = &AttachRunMarker{
			Requested: true,
			Status:    "pending",
			Message:   "receipt will be attached to the active run manifest",
		}
	}

	for _, gate := range selected {
		result := executeGate(gate, repoRoot, opts.DryRun)
		receipt.Results = append(receipt.Results, result)
		receipt.Summary.add(result.Status)
	}
	receipt.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if opts.AttachRun {
		if err := attachGateReceiptToRun(&receipt, environ); err != nil {
			return writeError("gates run", "attach-run-failed", err.Error(), opts.JSON, stdout, stderr, 1)
		}
	}

	ok := receipt.Summary.Failed == 0 && receipt.Summary.Blocked == 0 && receipt.Summary.TimedOut == 0 && receipt.Summary.Errors == 0
	if opts.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(gateRunResponse{OK: ok, Command: "gates run", Receipt: &receipt})
	} else {
		renderRunHuman(stdout, receipt, ok)
	}
	if ok {
		return 0
	}
	return 1
}

func attachGateReceiptToRun(receipt *RunReceipt, environ []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	store, bundle, _, err := runstate.ResolveStore(wd, environ, now)
	if err != nil {
		return err
	}
	manifest, err := store.LoadForTask(filepath.Base(bundle.Root))
	if err != nil {
		return err
	}
	receiptName := receipt.Gate
	if receiptName == "" {
		receiptName = receipt.Profile
	}
	if receiptName == "" {
		receiptName = "all"
	}
	receiptFile := safeReceiptName(receiptName) + ".json"
	if receipt.AttachRun != nil {
		receipt.AttachRun.Status = "attached"
		receipt.AttachRun.Message = "receipt appended to run manifest audit"
	}
	receipt.ReceiptPath = filepath.ToSlash(filepath.Join(store.ReceiptsDir(), "gates", receiptFile))
	content, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	receiptPath, err := store.WriteReceipt("gates", receiptFile, content)
	if err != nil {
		return err
	}
	receipt.ReceiptPath = filepath.ToSlash(receiptPath)
	status := "passed"
	if receipt.Summary.Failed > 0 {
		status = "failed"
	} else if receipt.Summary.Blocked > 0 {
		status = "blocked"
	} else if receipt.Summary.TimedOut > 0 {
		status = "timeout"
	} else if receipt.Summary.Errors > 0 {
		status = "error"
	} else if receipt.Summary.Skipped == receipt.Summary.Total {
		status = "skipped"
	}
	if err := manifest.AddAuditItem(runstate.AuditItem{
		Kind:        "gate",
		Name:        receiptName,
		Status:      status,
		Summary:     fmt.Sprintf("gates run total=%d passed=%d failed=%d blocked=%d timed_out=%d errors=%d", receipt.Summary.Total, receipt.Summary.Passed, receipt.Summary.Failed, receipt.Summary.Blocked, receipt.Summary.TimedOut, receipt.Summary.Errors),
		ReceiptPath: receipt.ReceiptPath,
	}, now); err != nil {
		return err
	}
	return store.Save(manifest)
}

func safeReceiptName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "gate"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(builder.String(), "-")
	if cleaned == "" {
		return "gate"
	}
	return cleaned
}

func loadRunnerSet(opts runOptions, environ []string) (Set, string, error) {
	if opts.ConfigPath != "" {
		set, err := LoadFile(opts.ConfigPath, "explicit config")
		if err != nil {
			return Set{}, "", err
		}
		configPath, err := filepath.Abs(opts.ConfigPath)
		if err != nil {
			return Set{}, "", err
		}
		return set, filepath.Dir(configPath), nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return Set{}, "", err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return Set{}, "", err
	}
	repoRoot := resolved.Context.RepositoryRoot
	if repoRoot == "" {
		repoRoot = resolved.Context.WorkingDirectory
	}
	if resolved.Config.Loaded {
		return setFromAdapterConfig(resolved.Config), repoRoot, nil
	}
	return EmptySet(resolved.Config.Path), repoRoot, nil
}

func selectRunnableGates(set Set, opts runOptions) ([]Gate, error) {
	if opts.GateName != "" {
		gate, ok := gateByName(set.Gates, opts.GateName)
		if !ok {
			return nil, fmt.Errorf("gate %q is not declared", opts.GateName)
		}
		return []Gate{gate}, nil
	}
	if opts.Profile != "" {
		profile, ok := set.Profiles[opts.Profile]
		if !ok {
			return nil, fmt.Errorf("gate profile %q is not declared", opts.Profile)
		}
		selected := make([]Gate, 0, len(profile.Required))
		for _, name := range profile.Required {
			gate, ok := gateByName(set.Gates, name)
			if !ok {
				return nil, fmt.Errorf("gate profile %q references missing gate %q", opts.Profile, name)
			}
			selected = append(selected, gate)
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("gate profile %q does not select any gates", opts.Profile)
		}
		return selected, nil
	}

	selected := []Gate{}
	for _, gate := range set.Gates {
		if gate.Type == "command" {
			selected = append(selected, gate)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no command gates are declared")
	}
	return selected, nil
}

func gateByName(gates []Gate, name string) (Gate, bool) {
	for _, gate := range gates {
		if gate.Name == name {
			return gate, true
		}
	}
	return Gate{}, false
}

func executeGate(gate Gate, repoRoot string, dryRun bool) RunResult {
	result := RunResult{
		Gate:        gate.Name,
		Type:        gate.Type,
		Destructive: gate.Destructive,
	}
	if gate.Command != nil {
		result.Argv = append([]string{}, gate.Command.Argv...)
	}
	timeout := gateTimeout(gate)
	result.TimeoutSeconds = int(timeout / time.Second)

	argv, cwd, reason := validateGateExecution(gate, repoRoot)
	if reason != "" {
		result.Status = "blocked"
		result.Reason = reason
		return result
	}
	result.WorkingDirectory = filepath.ToSlash(cwd)

	if dryRun {
		result.Status = "skipped"
		result.Reason = "dry-run"
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
	stdout := &limitedBuffer{cap: outputCapBytes}
	stderr := &limitedBuffer{cap: outputCapBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result.Stdout = stdout.snippet()
	result.Stderr = stderr.snippet()
	if cmd.ProcessState != nil {
		exitCode := cmd.ProcessState.ExitCode()
		result.ExitCode = &exitCode
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "timeout"
		result.Error = fmt.Sprintf("gate exceeded %ds timeout", result.TimeoutSeconds)
		return result
	}
	if err != nil {
		if result.ExitCode != nil {
			result.Status = "failed"
			return result
		}
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	result.Status = "passed"
	return result
}

func validateGateExecution(gate Gate, repoRoot string) ([]string, string, string) {
	if gate.Type != "command" {
		return nil, "", fmt.Sprintf("gate type %q is not executable", gate.Type)
	}
	if gate.Command == nil || len(gate.Command.Argv) == 0 {
		return nil, "", "command.argv is required for gates run"
	}
	if gate.Destructive {
		return nil, "", "destructive gates are blocked by default"
	}
	if containsShellExecution(gate.Command.Argv) {
		return nil, "", "shell execution is not allowed; declare a direct argv command"
	}
	cwd, err := resolveGateCWD(repoRoot, gate.Command.WorkingDirectory)
	if err != nil {
		return nil, "", err.Error()
	}
	return gate.Command.Argv, cwd, ""
}

func gateTimeout(gate Gate) time.Duration {
	if gate.Timeout == nil || gate.Timeout.Seconds <= 0 {
		return defaultRunTimeout
	}
	return time.Duration(gate.Timeout.Seconds) * time.Second
}

func containsShellExecution(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	executable := strings.ToLower(filepath.Base(argv[0]))
	switch executable {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	}
	for _, arg := range argv {
		if looksShellLike(arg) {
			return true
		}
	}
	return false
}

func resolveGateCWD(repoRoot string, workingDirectory string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	cwd := root
	if strings.TrimSpace(workingDirectory) != "" {
		if filepath.IsAbs(workingDirectory) {
			cwd = filepath.Clean(workingDirectory)
		} else {
			cwd = filepath.Clean(filepath.Join(root, workingDirectory))
		}
	}
	if !pathInside(root, cwd) {
		return "", fmt.Errorf("command working directory %q escapes repo root %q", cwd, root)
	}
	rootEval, rootErr := filepath.EvalSymlinks(root)
	cwdEval, cwdErr := filepath.EvalSymlinks(cwd)
	if rootErr != nil {
		return "", fmt.Errorf("resolve repo root: %w", rootErr)
	}
	if cwdErr != nil {
		return "", fmt.Errorf("resolve command working directory: %w", cwdErr)
	}
	if !pathInside(rootEval, cwdEval) {
		return "", fmt.Errorf("command working directory %q resolves outside repo root %q", cwd, root)
	}
	return cwd, nil
}

func pathInside(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func (summary *RunSummary) add(status string) {
	summary.Total++
	switch status {
	case "passed":
		summary.Passed++
	case "failed":
		summary.Failed++
	case "skipped":
		summary.Skipped++
	case "blocked":
		summary.Blocked++
	case "timeout":
		summary.TimedOut++
	default:
		summary.Errors++
	}
}

func renderRunHuman(stdout io.Writer, receipt RunReceipt, ok bool) {
	status := "blocked"
	if ok {
		status = "ok"
	}
	_, _ = fmt.Fprintf(stdout, "Anton gates run\n")
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", status)
	_, _ = fmt.Fprintf(stdout, "Config: %s (%s)\n", receipt.Source.Source, receipt.Source.Path)
	_, _ = fmt.Fprintf(stdout, "Gates: total=%d passed=%d failed=%d skipped=%d blocked=%d timeout=%d errors=%d\n", receipt.Summary.Total, receipt.Summary.Passed, receipt.Summary.Failed, receipt.Summary.Skipped, receipt.Summary.Blocked, receipt.Summary.TimedOut, receipt.Summary.Errors)
	for _, result := range receipt.Results {
		_, _ = fmt.Fprintf(stdout, "- %s: %s", result.Gate, result.Status)
		if result.Reason != "" {
			_, _ = fmt.Fprintf(stdout, " (%s)", result.Reason)
		}
		_, _ = fmt.Fprintln(stdout)
	}
	if receipt.AttachRun != nil {
		_, _ = fmt.Fprintf(stdout, "Attach run: %s - %s\n", receipt.AttachRun.Status, receipt.AttachRun.Message)
	}
}

type limitedBuffer struct {
	cap       int
	content   []byte
	total     int
	truncated bool
}

func (buffer *limitedBuffer) Write(p []byte) (int, error) {
	buffer.total += len(p)
	remaining := buffer.cap - len(buffer.content)
	if remaining > 0 {
		if len(p) <= remaining {
			buffer.content = append(buffer.content, p...)
		} else {
			buffer.content = append(buffer.content, p[:remaining]...)
			buffer.truncated = true
		}
	} else if len(p) > 0 {
		buffer.truncated = true
	}
	return len(p), nil
}

func (buffer *limitedBuffer) snippet() OutputSnippet {
	return OutputSnippet{
		Text:      string(buffer.content),
		Bytes:     buffer.total,
		Truncated: buffer.truncated,
	}
}
