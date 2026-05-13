package history

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type commandOptions struct {
	JSON        bool
	Limit       int
	SessionRoot string
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *responseData `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type responseData struct {
	Store          string    `json:"store"`
	WorkingRoot    string    `json:"working_root"`
	ReceiptCount   int       `json:"receipt_count"`
	AppendedCount  int       `json:"appended_count,omitempty"`
	Receipts       []Receipt `json:"receipts"`
	Warnings       []Warning `json:"warnings,omitempty"`
	SessionRoot    string    `json:"session_root,omitempty"`
	ScannedSources int       `json:"scanned_sources,omitempty"`
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
	case "show":
		return runShow(args[1:], stdout, stderr, environ)
	case "sync":
		return runSync(args[1:], stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown history command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runShow(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("history show", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	root, err := workingRoot(environ)
	if err != nil {
		return writeError("history show", "history-show-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	store := NewStore(root)
	result := store.Read()
	data := responseData{
		Store:        store.Path(),
		WorkingRoot:  root,
		ReceiptCount: len(result.Receipts),
		Receipts:     result.Receipts,
		Warnings:     result.Warnings,
	}
	exitCode := 0
	if hasFatalStoreWarning(result.Warnings) {
		exitCode = 1
	}
	return writeResponse("history show", data, opts.JSON, stdout, exitCode)
}

func runSync(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("history sync", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}
	root, err := workingRoot(environ)
	if err != nil {
		return writeError("history sync", "history-sync-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	archiveReceipts, archiveWarnings := scanCodexSessions(environ, ArchiveOptions{
		SessionRoot: opts.SessionRoot,
		FileLimit:   opts.Limit,
	})
	workReceipts, workWarnings := scanProjectWorkMemory(root, environ, WorkMemoryOptions{FileLimit: opts.Limit})
	candidates := append(archiveReceipts, workReceipts...)
	sortReceipts(candidates)

	store := NewStore(root)
	appended, storeWarnings := store.AppendNew(candidates)
	result := store.Read()
	warnings := append(archiveWarnings, workWarnings...)
	warnings = append(warnings, storeWarnings...)
	warnings = append(warnings, result.Warnings...)

	data := responseData{
		Store:          store.Path(),
		WorkingRoot:    root,
		ReceiptCount:   len(result.Receipts),
		AppendedCount:  appended,
		Receipts:       result.Receipts,
		Warnings:       warnings,
		SessionRoot:    firstNonEmpty(opts.SessionRoot, defaultCodexSessionRoot(environ)),
		ScannedSources: len(candidates),
	}
	exitCode := 0
	if hasFatalStoreWarning(warnings) {
		exitCode = 1
	}
	return writeResponse("history sync", data, opts.JSON, stdout, exitCode)
}

func workingRoot(environ []string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return wd, nil
		}
		current = parent
	}
}

func parseOptions(args []string) (commandOptions, error) {
	opts := commandOptions{Limit: defaultArchiveFileLimit}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--json":
			opts.JSON = true
		case "--limit":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--limit requires a value")
			}
			value, err := strconv.Atoi(args[index])
			if err != nil || value <= 0 {
				return opts, fmt.Errorf("--limit must be a positive integer")
			}
			opts.Limit = value
		case "--sessions-root":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("--sessions-root requires a value")
			}
			opts.SessionRoot = filepath.Clean(args[index])
		default:
			return opts, fmt.Errorf("unexpected argument %q", arg)
		}
	}
	return opts, nil
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
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
  anton history show [--json]
  anton history sync [--json] [--limit N] [--sessions-root PATH]

Native Anton history stores append-only evidence receipts at .anton/history/receipts.jsonl.
`
}

func writeResponse(command string, data responseData, jsonOutput bool, stdout io.Writer, exitCode int) int {
	if jsonOutput {
		payload := response{OK: exitCode == 0, Command: command, Data: &data}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stdout, "%s: %d receipts", command, data.ReceiptCount)
	if data.AppendedCount > 0 {
		_, _ = fmt.Fprintf(stdout, ", %d appended", data.AppendedCount)
	}
	if len(data.Warnings) > 0 {
		_, _ = fmt.Fprintf(stdout, ", %d warnings", len(data.Warnings))
	}
	_, _ = io.WriteString(stdout, "\n")
	return exitCode
}

func hasFatalStoreWarning(warnings []Warning) bool {
	for _, warning := range warnings {
		if warning.Code == receiptStoreSymlinkCode || warning.Code == "receipt-store-lstat-failed" {
			return true
		}
	}
	return false
}

func writeError(command string, code string, message string, jsonOutput bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	if jsonOutput {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
