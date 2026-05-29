package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"gopkg.in/yaml.v3"
)

type legacyInventory struct {
	Tasks  []LegacyTaskRecord
	Active []LegacyTaskRecord
}

type legacyPhysEditStatus struct {
	Task struct {
		ID        string `yaml:"id"`
		Topic     string `yaml:"topic"`
		Lifecycle string `yaml:"lifecycle"`
		Phase     string `yaml:"phase"`
	} `yaml:"task"`
}

func scanLegacyInventory(legacyRoot string, config adapter.Config) (legacyInventory, []Issue, error) {
	inventory := legacyInventory{
		Tasks:  []LegacyTaskRecord{},
		Active: []LegacyTaskRecord{},
	}
	issues := []Issue{}
	if !usesTopicLayerLegacy(config) {
		return inventory, issues, nil
	}

	info, err := os.Stat(legacyRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return inventory, issues, nil
		}
		return inventory, issues, fmt.Errorf("stat legacy tasks root %s: %w", legacyRoot, err)
	}
	if !info.IsDir() {
		return inventory, issues, fmt.Errorf("legacy tasks root is not a directory: %s", legacyRoot)
	}

	err = filepath.WalkDir(legacyRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		parts := legacyPathParts(legacyRoot, path)
		if len(parts) == 0 {
			return nil
		}
		if entry.IsDir() {
			if isCompletedLegacyPath(parts) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "status.yaml" || isCompletedLegacyPath(parts) {
			return nil
		}
		if hasArchiveLegacySegment(parts) {
			inventory.Tasks = append(inventory.Tasks, parseArchiveLegacyStatusMetadata(legacyRoot, path))
			return nil
		}
		topic, taskID, canonical := canonicalLegacyActivePath(parts)
		if !canonical {
			return nil
		}
		record, parseErr := parseLegacyPhysEditStatus(legacyRoot, path, topic, taskID)
		if parseErr != nil {
			issues = append(issues, Issue{
				Level:      "error",
				Code:       "state-dual-read-legacy-parse-failed",
				RuleID:     "state.dual_read.legacy_status_schema",
				File:       path,
				Message:    parseErr.Error(),
				RepairHint: "fix task.id, task.topic, task.lifecycle, or move non-current files out of tasks/active",
			})
			return nil
		}
		if strings.EqualFold(record.Lifecycle, "active") {
			record.Classification = "current_active"
			inventory.Tasks = append(inventory.Tasks, record)
			inventory.Active = append(inventory.Active, record)
			return nil
		}
		record.Classification = "legacy_active_dir_inactive"
		record.Reason = "task.lifecycle is not active; task.phase is advisory and does not affect active classification"
		inventory.Tasks = append(inventory.Tasks, record)
		return nil
	})
	if err != nil {
		return inventory, issues, err
	}
	return inventory, issues, nil
}

func parseLegacyPhysEditStatus(legacyRoot string, path string, pathTopic string, pathTaskID string) (LegacyTaskRecord, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return LegacyTaskRecord{}, err
	}
	payload := legacyPhysEditStatus{}
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return LegacyTaskRecord{}, err
	}
	taskID := strings.TrimSpace(payload.Task.ID)
	topic := strings.TrimSpace(payload.Task.Topic)
	lifecycle := strings.TrimSpace(payload.Task.Lifecycle)
	phase := strings.TrimSpace(payload.Task.Phase)
	if taskID == "" {
		return LegacyTaskRecord{}, errors.New("task.id is required")
	}
	if topic == "" {
		return LegacyTaskRecord{}, errors.New("task.topic is required")
	}
	if lifecycle == "" {
		return LegacyTaskRecord{}, errors.New("task.lifecycle is required")
	}
	if taskID != pathTaskID {
		return LegacyTaskRecord{}, fmt.Errorf("task.id %q does not match active task path id %q", taskID, pathTaskID)
	}
	normalizedTopic := normalizeLegacyTopic(topic)
	if normalizedTopic != pathTopic {
		return LegacyTaskRecord{}, fmt.Errorf("task.topic %q does not match topic-layer path topic %q", topic, pathTopic)
	}
	if err := adapter.ValidateTaskID(taskID); err != nil {
		return LegacyTaskRecord{}, err
	}
	return LegacyTaskRecord{
		TaskID:         taskID,
		Topic:          normalizedTopic,
		Lifecycle:      lifecycle,
		Phase:          phase,
		SourceFile:     filepath.ToSlash(relativeOrAbsolute(legacyRoot, path)),
		Classification: "legacy_active_dir_inactive",
	}, nil
}

func parseArchiveLegacyStatusMetadata(legacyRoot string, path string) LegacyTaskRecord {
	record := LegacyTaskRecord{
		SourceFile:     filepath.ToSlash(relativeOrAbsolute(legacyRoot, path)),
		Classification: "archive_or_history_only",
		Reason:         "path is under _legacy_* or _archived_worktree_rescue_* and is excluded from current active parity",
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return record
	}
	payload := legacyPhysEditStatus{}
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return record
	}
	record.TaskID = strings.TrimSpace(payload.Task.ID)
	record.Topic = strings.TrimSpace(payload.Task.Topic)
	record.Lifecycle = strings.TrimSpace(payload.Task.Lifecycle)
	record.Phase = strings.TrimSpace(payload.Task.Phase)
	return record
}

func legacyPathParts(root string, path string) []string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return nil
	}
	return strings.Split(filepath.ToSlash(relative), "/")
}

func canonicalLegacyActivePath(parts []string) (string, string, bool) {
	if len(parts) != 5 {
		return "", "", false
	}
	if parts[1] != "tasks" || parts[2] != "active" || parts[4] != "status.yaml" {
		return "", "", false
	}
	topic := strings.TrimSpace(parts[0])
	taskID := strings.TrimSpace(parts[3])
	if topic == "" || taskID == "" {
		return "", "", false
	}
	return topic, taskID, true
}

func hasArchiveLegacySegment(parts []string) bool {
	for _, part := range parts {
		if strings.HasPrefix(part, "_legacy_") || strings.HasPrefix(part, "_archived_worktree_rescue_") {
			return true
		}
	}
	return false
}

func isCompletedLegacyPath(parts []string) bool {
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] == "tasks" && parts[index+1] == "completed" {
			return true
		}
	}
	return false
}

func usesTopicLayerLegacy(config adapter.Config) bool {
	if config.Tasks.TopicLayer || strings.EqualFold(strings.TrimSpace(config.Tasks.Layout), "topic-layer") {
		return true
	}
	return filepath.Base(filepath.Clean(config.Tasks.Root)) == "project_progress"
}

func normalizeLegacyTopic(topic string) string {
	return strings.TrimPrefix(strings.TrimSpace(topic), "project_progress/")
}
