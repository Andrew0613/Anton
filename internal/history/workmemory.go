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

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	defaultWorkFileLimit = 120
	maxWorkFileBytes     = 128 * 1024
)

type WorkMemoryOptions struct {
	FileLimit int
}

func scanProjectWorkMemory(repoRoot string, environ []string, opts WorkMemoryOptions) ([]Receipt, []Warning) {
	limit := opts.FileLimit
	if limit <= 0 {
		limit = defaultWorkFileLimit
	}

	config, warnings := loadHistoryConfig(repoRoot, environ)
	tasksRoot := strings.TrimSpace(config.Tasks.Root)
	if tasksRoot == "" {
		tasksRoot = ".anton/tasks"
	}

	var receipts []Receipt
	var more []Receipt
	var scanWarnings []Warning

	more, scanWarnings = scanTaskBundles(repoRoot, tasksRoot, limit)
	receipts = append(receipts, more...)
	warnings = append(warnings, scanWarnings...)

	more, scanWarnings = scanMemoryEvents(repoRoot, limit)
	receipts = append(receipts, more...)
	warnings = append(warnings, scanWarnings...)

	more, scanWarnings = scanDeclaredWorkRoots(repoRoot, config.Extensions.History.WorkRecordRoots, limit)
	receipts = append(receipts, more...)
	warnings = append(warnings, scanWarnings...)

	return receipts, warnings
}

func loadHistoryConfig(repoRoot string, environ []string) (adapter.Config, []Warning) {
	context, err := adapter.DetectContext(repoRoot, environ)
	if err != nil {
		return adapter.Config{Tasks: adapter.TasksConfig{Root: ".anton/tasks"}}, []Warning{{Code: "history-config-read-failed", Message: err.Error(), Path: repoRoot}}
	}
	config, err := adapter.LoadConfig(context)
	if err != nil {
		return adapter.Config{Tasks: adapter.TasksConfig{Root: ".anton/tasks"}}, []Warning{{Code: "history-config-read-failed", Message: err.Error(), Path: repoRoot}}
	}
	return config, nil
}

func scanTaskBundles(repoRoot, tasksRoot string, limit int) ([]Receipt, []Warning) {
	root, ok, warning := safeRepoPath(repoRoot, tasksRoot, true)
	if !ok {
		if warning.Code == "" {
			return nil, nil
		}
		return nil, []Warning{warning}
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	var candidateFiles []string
	var warnings []Warning
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, Warning{Code: "working-memory-walk-failed", Message: err.Error(), Path: path})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			warnings = append(warnings, Warning{Code: "working-memory-symlink-refused", Message: "task bundle files must be regular repo-local files, not symlinks", Path: path})
			return nil
		}
		name := entry.Name()
		switch name {
		case "task_plan.md", "findings.md", "progress.md", "status.yaml":
			candidateFiles = append(candidateFiles, path)
		}
		return nil
	})
	if err != nil {
		warnings = append(warnings, Warning{Code: "working-memory-walk-failed", Message: err.Error(), Path: root})
	}
	sort.Strings(candidateFiles)
	if len(candidateFiles) > limit {
		warnings = append(warnings, Warning{
			Code:    "working-memory-scan-limited",
			Message: fmt.Sprintf("task bundle scan found %d files; limited to %d", len(candidateFiles), limit),
			Path:    root,
		})
		candidateFiles = candidateFiles[:limit]
	}

	return workFileReceipts(candidateFiles, "anton_task_bundle", "anton-task", warnings)
}

func scanMemoryEvents(repoRoot string, limit int) ([]Receipt, []Warning) {
	path := filepath.Join(repoRoot, ".anton", "memory", "events.jsonl")
	file, warning, err := openRegularWorkFile(path)
	if warning.Code != "" {
		return nil, []Warning{warning}
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Warning{{Code: "memory-events-read-failed", Message: err.Error(), Path: path}}
	}
	defer file.Close()

	var receipts []Receipt
	var warnings []Warning
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxWorkFileBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if lineNumber > limit {
			warnings = append(warnings, Warning{
				Code:    "working-memory-scan-limited",
				Message: fmt.Sprintf("memory event scan exceeded %d events", limit),
				Path:    path,
			})
			break
		}
		line := scanner.Bytes()
		var payload map[string]any
		if err := json.Unmarshal(line, &payload); err != nil {
			warnings = append(warnings, Warning{
				Code:    "malformed-memory-event",
				Message: fmt.Sprintf("line %d is not valid JSONL: %v", lineNumber, err),
				Path:    path,
			})
			continue
		}
		timestamp := fileModTime(path)
		if raw, ok := payload["timestamp"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				timestamp = parsed
			}
		}
		receipts = append(receipts, newReceipt("anton_memory_event", Source{Kind: "anton-memory", Path: fmt.Sprintf("%s:%d", path, lineNumber)}, timestamp, "medium", append([]byte(nil), line...), nil))
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, Warning{Code: "memory-events-scan-failed", Message: err.Error(), Path: path})
	}
	return receipts, warnings
}

func scanDeclaredWorkRoots(repoRoot string, roots []string, limit int) ([]Receipt, []Warning) {
	var receipts []Receipt
	var warnings []Warning
	for index, rootSpec := range roots {
		root, ok, warning := safeRepoPath(repoRoot, rootSpec, false)
		if !ok {
			if warning.Code == "" {
				continue
			}
			warnings = append(warnings, warning)
			continue
		}
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			warnings = append(warnings, Warning{
				Code:    "working-memory-missing-root",
				Message: "declared work-record root is missing",
				Path:    root,
			})
			continue
		}
		files, rootWarnings := declaredWorkFiles(root, limit)
		warnings = append(warnings, rootWarnings...)
		rootReceipts, rootWarnings := workFileReceipts(files, "declared_work_record", fmt.Sprintf("declared-work-root-%d", index), nil)
		receipts = append(receipts, rootReceipts...)
		warnings = append(warnings, rootWarnings...)
	}
	return receipts, warnings
}

func declaredWorkFiles(root string, limit int) ([]string, []Warning) {
	var files []string
	var warnings []Warning
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, Warning{Code: "working-memory-walk-failed", Message: err.Error(), Path: path})
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, evalErr := filepath.EvalSymlinks(path)
			if evalErr != nil {
				warnings = append(warnings, Warning{Code: "working-memory-symlink-refused", Message: evalErr.Error(), Path: path})
				return nil
			}
			rootEval, _ := filepath.EvalSymlinks(root)
			if !isWithin(rootEval, target) {
				warnings = append(warnings, Warning{Code: "working-memory-symlink-escape", Message: "symlink target escapes declared work-record root", Path: path})
				return nil
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		warnings = append(warnings, Warning{Code: "working-memory-walk-failed", Message: err.Error(), Path: root})
	}
	sort.Strings(files)
	if len(files) > limit {
		warnings = append(warnings, Warning{
			Code:    "working-memory-scan-limited",
			Message: fmt.Sprintf("declared work root found %d files; limited to %d", len(files), limit),
			Path:    root,
		})
		files = files[:limit]
	}
	return files, warnings
}

func workFileReceipts(files []string, receiptType, sourceKind string, warnings []Warning) ([]Receipt, []Warning) {
	var receipts []Receipt
	for _, path := range files {
		content, warning, err := readRegularWorkFile(path)
		if warning.Code != "" {
			warnings = append(warnings, warning)
			continue
		}
		if err != nil {
			warnings = append(warnings, Warning{Code: "working-memory-read-failed", Message: err.Error(), Path: path})
			continue
		}
		if len(content) > maxWorkFileBytes {
			warnings = append(warnings, Warning{
				Code:    "working-memory-oversized-payload",
				Message: fmt.Sprintf("file exceeds %d byte payload cap", maxWorkFileBytes),
				Path:    path,
			})
			content = content[:maxWorkFileBytes]
		}
		if filepath.Base(path) == "status.yaml" {
			if malformedStatusYAML(content) {
				warnings = append(warnings, Warning{Code: "malformed-status", Message: "status.yaml contains a non-comment line without a YAML key", Path: path})
			}
		}
		receipts = append(receipts, newReceipt(receiptType, Source{Kind: sourceKind, Path: path}, fileModTime(path), "high", content, nil))
	}
	return receipts, warnings
}

func readRegularWorkFile(path string) ([]byte, Warning, error) {
	_, warning, err := lstatRegularWorkFile(path)
	if warning.Code != "" || err != nil {
		return nil, warning, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, Warning{}, err
	}
	return content, Warning{}, nil
}

func openRegularWorkFile(path string) (*os.File, Warning, error) {
	_, warning, err := lstatRegularWorkFile(path)
	if warning.Code != "" || err != nil {
		return nil, warning, err
	}
	file, err := os.Open(path)
	return file, Warning{}, err
}

func lstatRegularWorkFile(path string) (os.FileInfo, Warning, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, Warning{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, Warning{Code: "working-memory-symlink-refused", Message: "working memory files must be regular repo-local files, not symlinks", Path: path}, nil
	}
	if !info.Mode().IsRegular() {
		return nil, Warning{Code: "working-memory-nonregular-refused", Message: "working memory files must be regular files", Path: path}, nil
	}
	return info, Warning{}, nil
}

func malformedStatusYAML(content []byte) bool {
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "- ") {
			continue
		}
		if !strings.Contains(line, ":") {
			return true
		}
	}
	return false
}

func safeRepoPath(repoRoot, raw string, missingIsSilent bool) (string, bool, Warning) {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "" || cleaned == "." {
		if missingIsSilent {
			return "", false, Warning{}
		}
		return "", false, Warning{Code: "working-memory-invalid-root", Message: "declared work-record root must not be empty"}
	}
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", false, Warning{
			Code:    "working-memory-root-refused",
			Message: "work-record roots must be repo-relative and stay inside the repository",
			Path:    raw,
		}
	}
	full := filepath.Join(repoRoot, cleaned)
	evaluatedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		evaluatedRepo = repoRoot
	}
	evaluatedFull := full
	if existing, err := filepath.EvalSymlinks(full); err == nil {
		evaluatedFull = existing
	}
	if !isWithin(evaluatedRepo, evaluatedFull) {
		return "", false, Warning{
			Code:    "working-memory-symlink-escape",
			Message: "work-record root escapes repository after symlink resolution",
			Path:    full,
		}
	}
	return full, true, Warning{}
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func fileModTime(path string) time.Time {
	if stat, err := os.Stat(path); err == nil {
		return stat.ModTime()
	}
	return time.Now().UTC()
}
