package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
)

type options struct {
	JSON bool

	ID          string
	Title       string
	Status      string
	Note        string
	Kind        string
	Name        string
	Summary     string
	ReceiptPath string
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *responseData `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type responseData struct {
	Adapter          string           `json:"adapter"`
	WorkingDirectory string           `json:"working_directory"`
	Config           configContract   `json:"config"`
	BundleRoot       string           `json:"bundle_root"`
	ManifestPath     string           `json:"manifest_path"`
	TaskID           string           `json:"task_id"`
	ChecklistSummary ChecklistSummary `json:"checklist_summary"`
	Manifest         *Manifest        `json:"manifest,omitempty"`
}

type configContract struct {
	Path           string `json:"path"`
	Source         string `json:"source"`
	TasksRoot      string `json:"tasks_root"`
	PlanningMode   string `json:"planning_mode"`
	RunManifest    string `json:"run_manifest"`
	RunReceiptsDir string `json:"run_receipts_dir"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}
	if len(args) > 1 && hasHelp(args[1:]) {
		_, _ = io.WriteString(stdout, usageText())
		return 0
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr, environ)
	case "status":
		return runStatus(args[1:], stdout, stderr, environ)
	case "task":
		return runTask(args[1:], stdout, stderr, environ)
	case "audit":
		return runAudit(args[1:], stdout, stderr, environ)
	case "close":
		return runClose(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown run command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runInit(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("run init", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	now := nowFromEnvironment(environ)
	store, bundle, resolved, err := ResolveStore(mustGetwd(), environ, now)
	if err != nil {
		return writeTaskBundleError("run init", err, opts.JSON, stdout, stderr)
	}
	manifest, err := store.Init(taskIDFromBundle(bundle), now)
	if err != nil {
		return writeError("run init", "run-init-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	return writeResponse("run init", buildResponseData(resolved, bundle, store, manifest), opts.JSON, stdout)
}

func runStatus(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("run status", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	now := nowFromEnvironment(environ)
	store, bundle, resolved, err := ResolveStore(mustGetwd(), environ, now)
	if err != nil {
		return writeTaskBundleError("run status", err, opts.JSON, stdout, stderr)
	}
	manifest, err := store.LoadForTask(taskIDFromBundle(bundle))
	if err != nil {
		return writeError("run status", "run-status-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	return writeResponse("run status", buildResponseData(resolved, bundle, store, manifest), opts.JSON, stdout)
}

func runTask(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		jsonOutput := hasJSON(args)
		return writeError("run task", "usage", "missing task subcommand", jsonOutput, stdout, stderr, 2)
	}
	switch args[0] {
	case "list":
		return runTaskList(args[1:], stdout, stderr, environ)
	case "add":
		return runTaskAdd(args[1:], stdout, stderr, environ)
	case "set":
		return runTaskSet(args[1:], stdout, stderr, environ)
	default:
		jsonOutput := hasJSON(args[1:])
		return writeError("run task", "usage", fmt.Sprintf("unsupported task subcommand: %s", args[0]), jsonOutput, stdout, stderr, 2)
	}
}

func runTaskList(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("run task list", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	return loadAndRespond("run task list", opts.JSON, stdout, stderr, environ)
}

func runTaskAdd(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseTaskAddOptions(args)
	if err != nil {
		return writeError("run task add", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	return mutateManifest("run task add", opts.JSON, stdout, stderr, environ, func(manifest *Manifest, now time.Time) error {
		return manifest.AddChecklistItem(opts.ID, opts.Title, now)
	})
}

func runTaskSet(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseTaskSetOptions(args)
	if err != nil {
		return writeError("run task set", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	result := mutateManifest("run task set", opts.JSON, stdout, stderr, environ, func(manifest *Manifest, now time.Time) error {
		return manifest.SetChecklistItem(opts.ID, opts.Status, opts.Note, now)
	})
	if result == 0 {
		// best-effort event log — don't fail the command if event log fails
		now := nowFromEnvironment(environ)
		store, _, _, storeErr := ResolveStore(mustGetwd(), environ, now)
		if storeErr == nil {
			data, _ := json.Marshal(map[string]string{"task_id": opts.ID, "status": opts.Status})
			_ = AppendEvent(store.EventLogPath(), Event{
				Ts:    now.Format(time.RFC3339),
				Event: "task_status_change",
				Data:  json.RawMessage(data),
			})
		}
	}
	return result
}

func runAudit(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		jsonOutput := hasJSON(args)
		return writeError("run audit", "usage", "expected audit subcommand: add | events", jsonOutput, stdout, stderr, 2)
	}
	switch args[0] {
	case "add":
		opts, err := parseAuditAddOptions(args[1:])
		if err != nil {
			return writeError("run audit add", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
		}
		return mutateManifest("run audit add", opts.JSON, stdout, stderr, environ, func(manifest *Manifest, now time.Time) error {
			return manifest.AddAuditItem(AuditItem{
				Kind:        opts.Kind,
				Name:        opts.Name,
				Status:      opts.Status,
				Summary:     opts.Summary,
				ReceiptPath: opts.ReceiptPath,
			}, now)
		})
	case "events":
		return runAuditEvents(args[1:], stdout, stderr, environ)
	default:
		jsonOutput := hasJSON(args[1:])
		return writeError("run audit", "usage", fmt.Sprintf("unknown audit subcommand: %s", args[0]), jsonOutput, stdout, stderr, 2)
	}
}

func runAuditEvents(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	jsonOutput := hasJSON(args)
	now := nowFromEnvironment(environ)
	store, _, _, err := ResolveStore(mustGetwd(), environ, now)
	if err != nil {
		return writeError("run audit events", "resolve-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	events, err := ReadEvents(store.EventLogPath())
	if err != nil {
		return writeError("run audit events", "read-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	if jsonOutput {
		_ = json.NewEncoder(stdout).Encode(events)
		return 0
	}
	for _, e := range events {
		_, _ = fmt.Fprintf(stdout, "[%s] %s\n", e.Ts, e.Event)
	}
	return 0
}

func runClose(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseCloseOptions(args)
	if err != nil {
		return writeError("run close", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	return mutateManifest("run close", opts.JSON, stdout, stderr, environ, func(manifest *Manifest, now time.Time) error {
		return manifest.CloseRun(opts.Status, opts.Summary, now)
	})
}

func loadAndRespond(command string, jsonOutput bool, stdout io.Writer, stderr io.Writer, environ []string) int {
	now := nowFromEnvironment(environ)
	store, bundle, resolved, err := ResolveStore(mustGetwd(), environ, now)
	if err != nil {
		return writeTaskBundleError(command, err, jsonOutput, stdout, stderr)
	}
	manifest, err := store.LoadForTask(taskIDFromBundle(bundle))
	if err != nil {
		return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	return writeResponse(command, buildResponseData(resolved, bundle, store, manifest), jsonOutput, stdout)
}

func mutateManifest(command string, jsonOutput bool, stdout io.Writer, stderr io.Writer, environ []string, mutate func(*Manifest, time.Time) error) int {
	now := nowFromEnvironment(environ)
	store, bundle, resolved, err := ResolveStore(mustGetwd(), environ, now)
	if err != nil {
		return writeTaskBundleError(command, err, jsonOutput, stdout, stderr)
	}
	manifest, err := store.LoadForTask(taskIDFromBundle(bundle))
	if err != nil {
		return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	if err := mutate(&manifest, now); err != nil {
		return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	if err := store.Save(manifest); err != nil {
		return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), jsonOutput, stdout, stderr, 1)
	}
	return writeResponse(command, buildResponseData(resolved, bundle, store, manifest), jsonOutput, stdout)
}

func buildResponseData(resolved adapter.Resolved, bundle adapter.ResolvedTaskBundle, store Store, manifest Manifest) responseData {
	return responseData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: resolved.Context.WorkingDirectory,
		Config: configContract{
			Path:           resolved.Config.Path,
			Source:         resolved.Config.Source(),
			TasksRoot:      resolved.Config.Tasks.Root,
			PlanningMode:   resolved.Config.PlanningMode(),
			RunManifest:    resolved.Config.RunManifestName(),
			RunReceiptsDir: resolved.Config.RunReceiptsDir(),
		},
		BundleRoot:       bundle.Root,
		ManifestPath:     store.Path(),
		TaskID:           manifest.TaskID,
		ChecklistSummary: manifest.ChecklistSummary(),
		Manifest:         &manifest,
	}
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.JSON = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func parseTaskAddOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--id":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --id")
			}
			opts.ID = strings.TrimSpace(args[index])
		case "--title":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --title")
			}
			opts.Title = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.ID == "" {
		return opts, fmt.Errorf("--id is required")
	}
	if opts.Title == "" {
		return opts, fmt.Errorf("--title is required")
	}
	return opts, nil
}

func parseTaskSetOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--id":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --id")
			}
			opts.ID = strings.TrimSpace(args[index])
		case "--status":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --status")
			}
			opts.Status = strings.TrimSpace(args[index])
		case "--note":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --note")
			}
			opts.Note = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.ID == "" {
		return opts, fmt.Errorf("--id is required")
	}
	if opts.Status == "" {
		return opts, fmt.Errorf("--status is required")
	}
	return opts, nil
}

func parseAuditAddOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--kind":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --kind")
			}
			opts.Kind = strings.TrimSpace(args[index])
		case "--name":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --name")
			}
			opts.Name = strings.TrimSpace(args[index])
		case "--status":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --status")
			}
			opts.Status = strings.TrimSpace(args[index])
		case "--summary":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --summary")
			}
			opts.Summary = strings.TrimSpace(args[index])
		case "--receipt-path":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --receipt-path")
			}
			opts.ReceiptPath = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	for flag, value := range map[string]string{
		"--kind":   opts.Kind,
		"--name":   opts.Name,
		"--status": opts.Status,
	} {
		if value == "" {
			return opts, fmt.Errorf("%s is required", flag)
		}
	}
	return opts, nil
}

func parseCloseOptions(args []string) (options, error) {
	opts := options{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			opts.JSON = true
		case "--status":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --status")
			}
			opts.Status = strings.TrimSpace(args[index])
		case "--summary":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --summary")
			}
			opts.Summary = strings.TrimSpace(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.Status == "" {
		return opts, fmt.Errorf("--status is required")
	}
	return opts, nil
}

func writeResponse(command string, data responseData, asJSON bool, stdout io.Writer) int {
	if asJSON {
		payload := response{OK: true, Command: command, Data: &data}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "%s: %s (%s)\n", command, data.TaskID, data.ManifestPath)
	return 0
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	if asJSON {
		payload := response{
			OK:      false,
			Command: command,
			Error: &errorPayload{
				Code:    code,
				Message: message,
			},
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", code, message)
	return exitCode
}

func writeTaskBundleError(command string, err error, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	var taskIdentityErr adapter.TaskIdentityRequiredError
	if errors.As(err, &taskIdentityErr) {
		return writeError(command, "task-identity-required", taskIdentityErr.Error(), asJSON, stdout, stderr, 1)
	}
	return writeError(command, strings.ReplaceAll(command, " ", "-")+"-failed", err.Error(), asJSON, stdout, stderr, 1)
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func hasJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton run init [--json]
  anton run status [--json]
  anton run task list [--json]
  anton run task add --id ID --title TITLE [--json]
  anton run task set --id ID --status STATUS [--note NOTE] [--json]
  anton run audit add --kind KIND --name NAME --status STATUS [--summary SUMMARY] [--receipt-path PATH] [--json]
  anton run close --status STATUS [--summary SUMMARY] [--json]

Run manifests are passive sidecars stored as run.json in an existing Anton task bundle.
`
}

func nowFromEnvironment(environ []string) time.Time {
	values := envMap(environ)
	if value := strings.TrimSpace(values["ANTON_RUN_NOW"]); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func envMap(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func taskIDFromBundle(bundle adapter.ResolvedTaskBundle) string {
	return strings.TrimSpace(filepath.Base(bundle.Root))
}
