package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultArchiveFileLimit = 40
	maxArchiveLineBytes     = 64 * 1024
)

type ArchiveOptions struct {
	SessionRoot string
	FileLimit   int
}

type archiveEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   any    `json:"message"`
	Payload   any    `json:"payload"`
}

func scanCodexSessions(environ []string, opts ArchiveOptions) ([]Receipt, []Warning) {
	root := opts.SessionRoot
	if strings.TrimSpace(root) == "" {
		root = defaultCodexSessionRoot(environ)
	}
	limit := opts.FileLimit
	if limit <= 0 {
		limit = defaultArchiveFileLimit
	}

	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, []Warning{{
			Code:    "missing-sessions-root",
			Message: "local Codex session root is missing; archive sync produced no session receipts",
			Path:    root,
		}}
	}
	if err != nil {
		return nil, []Warning{{
			Code:    "sessions-root-stat-failed",
			Message: err.Error(),
			Path:    root,
		}}
	}
	if !info.IsDir() {
		return nil, []Warning{{
			Code:    "sessions-root-not-directory",
			Message: "local Codex session root is not a directory",
			Path:    root,
		}}
	}

	files, warnings := sessionFiles(root, limit)
	var receipts []Receipt
	for _, path := range files {
		receipt, fileWarnings, ok := sessionReceipt(path)
		warnings = append(warnings, fileWarnings...)
		if ok {
			receipts = append(receipts, receipt)
		}
	}
	return receipts, warnings
}

func defaultCodexSessionRoot(environ []string) string {
	values := envMap(environ)
	if codexHome := strings.TrimSpace(values["CODEX_HOME"]); codexHome != "" {
		return filepath.Join(codexHome, "sessions")
	}
	if home := strings.TrimSpace(values["HOME"]); home != "" {
		return filepath.Join(home, ".codex", "sessions")
	}
	return filepath.Join(".", ".codex", "sessions")
}

func sessionFiles(root string, limit int) ([]string, []Warning) {
	var files []string
	var warnings []Warning
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, Warning{Code: "session-walk-failed", Message: err.Error(), Path: path})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		warnings = append(warnings, Warning{Code: "session-walk-failed", Message: err.Error(), Path: root})
	}
	sort.Strings(files)
	if len(files) > limit {
		warnings = append(warnings, Warning{
			Code:    "archive-scan-limited",
			Message: fmt.Sprintf("archive scan found %d session files; limited to %d", len(files), limit),
			Path:    root,
		})
		files = files[len(files)-limit:]
	}
	return files, warnings
}

func sessionReceipt(path string) (Receipt, []Warning, bool) {
	file, err := os.Open(path)
	if err != nil {
		return Receipt{}, []Warning{{Code: "session-read-failed", Message: err.Error(), Path: path}}, false
	}
	defer file.Close()

	var warnings []Warning
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxArchiveLineBytes)
	var events []string
	var timestamp time.Time
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if len(line) > maxArchiveLineBytes {
			warnings = append(warnings, Warning{
				Code:    "archive-oversized-payload",
				Message: fmt.Sprintf("line %d exceeds archive payload cap", lineNumber),
				Path:    path,
			})
			continue
		}
		var event archiveEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			warnings = append(warnings, Warning{
				Code:    "malformed-session",
				Message: fmt.Sprintf("line %d is not valid JSONL: %v", lineNumber, err),
				Path:    path,
			})
			continue
		}
		if timestamp.IsZero() && event.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
				timestamp = parsed
			}
		}
		text := archiveEventText(event)
		if text != "" {
			events = append(events, text)
		}
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, Warning{
			Code:    "archive-oversized-payload",
			Message: err.Error(),
			Path:    path,
		})
	}
	if len(events) == 0 {
		if len(warnings) == 0 {
			warnings = append(warnings, Warning{
				Code:    "empty-session",
				Message: "session archive had no readable events",
				Path:    path,
			})
		}
		return Receipt{}, warnings, false
	}
	if timestamp.IsZero() {
		if stat, err := os.Stat(path); err == nil {
			timestamp = stat.ModTime()
		}
	}

	content := []byte(strings.Join(events, "\n"))
	receipt := newReceipt("codex_session", Source{Kind: "codex-session", Path: path}, timestamp, "medium", content, map[string]string{
		"event_count": fmt.Sprintf("%d", len(events)),
	})
	return receipt, warnings, true
}

func archiveEventText(event archiveEvent) string {
	parts := []string{}
	if event.Type != "" {
		parts = append(parts, event.Type)
	}
	if event.Message != nil {
		parts = append(parts, jsonish(event.Message))
	}
	if event.Payload != nil {
		parts = append(parts, jsonish(event.Payload))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func jsonish(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		content, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(content)
	}
}
