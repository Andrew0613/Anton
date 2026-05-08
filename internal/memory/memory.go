package memory

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	eventSchemaVersion = "anton.memory.event.v1"
	defaultStaleAfter  = 30 * 24 * time.Hour
)

var validConfidences = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
}

type Event struct {
	SchemaVersion string `json:"schema_version"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Source        string `json:"source"`
	Freshness     string `json:"freshness"`
	Confidence    string `json:"confidence"`
	Author        string `json:"author,omitempty"`
	RecordedAt    string `json:"recorded_at"`
	Advisory      bool   `json:"advisory"`
}

type StatusEntry struct {
	Event
	FreshnessStatus string   `json:"freshness_status"`
	Warnings        []string `json:"warnings,omitempty"`
}

type Warning struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
}

type Summary struct {
	Status        string `json:"status"`
	Missing       bool   `json:"missing"`
	EntryCount    int    `json:"entry_count"`
	StaleCount    int    `json:"stale_count"`
	ConflictCount int    `json:"conflict_count"`
	WarningCount  int    `json:"warning_count"`
}

type ConfigData struct {
	Path           string `json:"path"`
	Source         string `json:"source"`
	EntrypointPath string `json:"entrypoint_path"`
	TasksRoot      string `json:"tasks_root"`
}

type StatusData struct {
	Adapter          string        `json:"adapter"`
	WorkingDirectory string        `json:"working_directory"`
	Config           ConfigData    `json:"config"`
	MemoryPath       string        `json:"memory_path"`
	Entries          []StatusEntry `json:"entries"`
	Warnings         []Warning     `json:"warnings,omitempty"`
	Summary          Summary       `json:"summary"`
}

type UpdateData struct {
	Adapter          string     `json:"adapter"`
	WorkingDirectory string     `json:"working_directory"`
	Config           ConfigData `json:"config"`
	MemoryPath       string     `json:"memory_path"`
	Event            Event      `json:"event"`
	Summary          Summary    `json:"summary"`
}

type CorruptError struct {
	Path string
	Line int
	Err  error
}

func (err CorruptError) Error() string {
	if err.Line > 0 {
		return fmt.Sprintf("corrupt memory log at %s line %d: %v", err.Path, err.Line, err.Err)
	}
	return fmt.Sprintf("corrupt memory log at %s: %v", err.Path, err.Err)
}

func (err CorruptError) Unwrap() error {
	return err.Err
}

type storagePaths struct {
	Base       string
	MemoryDir  string
	EventsPath string
}

func collectStatus(environ []string, now time.Time) (StatusData, error) {
	wd, resolved, paths, err := resolveMemoryRuntime(environ)
	if err != nil {
		return StatusData{}, err
	}

	events, missing, err := readEvents(paths.EventsPath)
	if err != nil {
		return StatusData{}, err
	}

	authority := authoritativeValues(resolved)
	entries := make([]StatusEntry, 0, len(events))
	warnings := []Warning{}
	staleCount := 0
	conflictCount := 0
	for _, event := range events {
		entry := StatusEntry{
			Event:           event,
			FreshnessStatus: freshnessStatus(event.Freshness, now),
		}
		if entry.FreshnessStatus == "stale" {
			staleCount++
			entry.Warnings = append(entry.Warnings, fmt.Sprintf("memory freshness %s is older than %s", event.Freshness, defaultStaleAfter))
			warnings = append(warnings, Warning{
				Level:   "warning",
				Code:    "memory-stale",
				Key:     event.Key,
				Message: fmt.Sprintf("memory key %q is stale; treat it as advisory context only", event.Key),
			})
		}
		if authoritative, ok := authority[event.Key]; ok && strings.TrimSpace(event.Value) != authoritative {
			conflictCount++
			message := fmt.Sprintf("memory value %q conflicts with authoritative %s %q; memory is advisory", event.Value, event.Key, authoritative)
			entry.Warnings = append(entry.Warnings, message)
			warnings = append(warnings, Warning{
				Level:   "warning",
				Code:    "memory-conflict-" + sanitizeCode(event.Key),
				Key:     event.Key,
				Message: message,
			})
		}
		entries = append(entries, entry)
	}

	return StatusData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           configData(resolved),
		MemoryPath:       paths.EventsPath,
		Entries:          entries,
		Warnings:         warnings,
		Summary: Summary{
			Status:        "ok",
			Missing:       missing,
			EntryCount:    len(entries),
			StaleCount:    staleCount,
			ConflictCount: conflictCount,
			WarningCount:  len(warnings),
		},
	}, nil
}

func appendEvent(environ []string, input updateOptions, now time.Time) (UpdateData, error) {
	wd, resolved, paths, err := resolveMemoryRuntime(environ)
	if err != nil {
		return UpdateData{}, err
	}
	if err := ensureMemoryPath(paths); err != nil {
		return UpdateData{}, err
	}

	freshness := input.Freshness
	if strings.TrimSpace(freshness) == "" {
		freshness = now.UTC().Format(time.RFC3339)
	}
	event := Event{
		SchemaVersion: eventSchemaVersion,
		Key:           strings.TrimSpace(input.Key),
		Value:         strings.TrimSpace(input.Value),
		Source:        strings.TrimSpace(input.Source),
		Freshness:     freshness,
		Confidence:    strings.TrimSpace(input.Confidence),
		Author:        strings.TrimSpace(input.Author),
		RecordedAt:    now.UTC().Format(time.RFC3339),
		Advisory:      true,
	}
	if event.Confidence == "" {
		event.Confidence = "medium"
	}
	if event.Author == "" {
		event.Author = defaultAuthor(environ)
	}
	if err := validateEvent(event); err != nil {
		return UpdateData{}, err
	}

	content, err := json.Marshal(event)
	if err != nil {
		return UpdateData{}, fmt.Errorf("marshal memory event: %w", err)
	}
	file, err := os.OpenFile(paths.EventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return UpdateData{}, fmt.Errorf("open %s for append: %w", paths.EventsPath, err)
	}
	defer file.Close()
	if _, err := file.Write(append(content, '\n')); err != nil {
		return UpdateData{}, fmt.Errorf("append %s: %w", paths.EventsPath, err)
	}

	return UpdateData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config:           configData(resolved),
		MemoryPath:       paths.EventsPath,
		Event:            event,
		Summary: Summary{
			Status:     "ok",
			EntryCount: 1,
		},
	}, nil
}

func readEvents(path string) ([]Event, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("refusing to read symlinked memory log: %s", path)
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("%s is a directory, expected memory JSONL file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	events := []Event{}
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			return nil, false, CorruptError{Path: path, Line: line, Err: err}
		}
		if err := validateEvent(event); err != nil {
			return nil, false, CorruptError{Path: path, Line: line, Err: err}
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("scan %s: %w", path, err)
	}
	return events, false, nil
}

func validateEvent(event Event) error {
	if event.SchemaVersion != eventSchemaVersion {
		return fmt.Errorf("schema_version must be %q", eventSchemaVersion)
	}
	if strings.TrimSpace(event.Key) == "" {
		return fmt.Errorf("key is required")
	}
	if strings.TrimSpace(event.Value) == "" {
		return fmt.Errorf("value is required")
	}
	if strings.TrimSpace(event.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if _, err := time.Parse(time.RFC3339, event.Freshness); err != nil {
		return fmt.Errorf("freshness must be RFC3339: %w", err)
	}
	if !validConfidences[event.Confidence] {
		return fmt.Errorf("confidence must be one of: low, medium, high")
	}
	if strings.TrimSpace(event.RecordedAt) == "" {
		return fmt.Errorf("recorded_at is required")
	}
	if _, err := time.Parse(time.RFC3339, event.RecordedAt); err != nil {
		return fmt.Errorf("recorded_at must be RFC3339: %w", err)
	}
	if !event.Advisory {
		return fmt.Errorf("advisory must be true")
	}
	return nil
}

func resolveMemoryRuntime(environ []string) (string, adapter.Resolved, storagePaths, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", adapter.Resolved{}, storagePaths{}, fmt.Errorf("get working directory: %w", err)
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return "", adapter.Resolved{}, storagePaths{}, err
	}
	base := resolved.Context.WorkingDirectory
	if resolved.Context.RepositoryRoot != "" {
		base = resolved.Context.RepositoryRoot
	}
	paths := storagePaths{
		Base:       base,
		MemoryDir:  filepath.Join(base, ".anton", "memory"),
		EventsPath: filepath.Join(base, ".anton", "memory", "events.jsonl"),
	}
	return wd, resolved, paths, nil
}

func ensureMemoryPath(paths storagePaths) error {
	antonDir := filepath.Join(paths.Base, ".anton")
	if err := ensureNoSymlinkDir(antonDir); err != nil {
		return err
	}
	if err := ensureNoSymlinkDir(paths.MemoryDir); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.MemoryDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", paths.MemoryDir, err)
	}
	if err := ensureNoSymlinkFile(paths.EventsPath); err != nil {
		return err
	}
	baseReal, err := filepath.EvalSymlinks(paths.Base)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", paths.Base, err)
	}
	dirReal, err := filepath.EvalSymlinks(paths.MemoryDir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", paths.MemoryDir, err)
	}
	if !pathWithinRoot(baseReal, dirReal) {
		return fmt.Errorf("memory path escapes repository root: %s", paths.MemoryDir)
	}
	return nil
}

func ensureNoSymlinkDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use symlinked memory directory: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is a file, expected memory directory", path)
	}
	return nil
}

func ensureNoSymlinkFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to append to symlinked memory log: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, expected memory JSONL file", path)
	}
	return nil
}

func authoritativeValues(resolved adapter.Resolved) map[string]string {
	context := resolved.Context
	config := resolved.Config
	entrypointPath := resolved.Definition.EntrypointPath(context)
	values := map[string]string{
		"entrypoint.path":                         strings.TrimSpace(config.Entrypoint.Path),
		"config.entrypoint_path":                  strings.TrimSpace(entrypointPath),
		"tasks.root":                              strings.TrimSpace(config.Tasks.Root),
		"config.tasks_root":                       strings.TrimSpace(config.Tasks.Root),
		"threads.default_project_strategy":        strings.TrimSpace(config.Threads.DefaultProjectStrategy),
		"contract.context.working_directory":      strings.TrimSpace(context.WorkingDirectory),
		"contract.context.repository_root":        strings.TrimSpace(context.RepositoryRoot),
		"contract.context.git_branch":             strings.TrimSpace(context.GitBranch),
		"contract.config.entrypoint_path":         strings.TrimSpace(entrypointPath),
		"contract.config.tasks_root":              strings.TrimSpace(config.Tasks.Root),
		"contract.config.threads_default_project": strings.TrimSpace(config.Threads.DefaultProjectStrategy),
	}
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}
	return values
}

func configData(resolved adapter.Resolved) ConfigData {
	return ConfigData{
		Path:           resolved.Config.Path,
		Source:         resolved.Config.Source(),
		EntrypointPath: resolved.Definition.EntrypointPath(resolved.Context),
		TasksRoot:      resolved.Config.Tasks.Root,
	}
}

func freshnessStatus(freshness string, now time.Time) string {
	parsed, err := time.Parse(time.RFC3339, freshness)
	if err != nil {
		return "stale"
	}
	if now.UTC().Sub(parsed.UTC()) > defaultStaleAfter {
		return "stale"
	}
	return "fresh"
}

func defaultAuthor(environ []string) string {
	values := envMap(environ)
	for _, key := range []string{"ANTON_AUTHOR", "USER", "USERNAME"} {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return "unknown"
}

func envMap(environ []string) map[string]string {
	values := map[string]string{}
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func sanitizeCode(value string) string {
	replacer := strings.NewReplacer(".", "-", "_", "-", "/", "-", " ", "-")
	return replacer.Replace(strings.ToLower(value))
}

func pathWithinRoot(root string, path string) bool {
	rootClean := filepath.Clean(root)
	pathClean := filepath.Clean(path)
	relative, err := filepath.Rel(rootClean, pathClean)
	if err != nil {
		return false
	}
	if relative == "." {
		return true
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(relative)
}
