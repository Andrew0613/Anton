package run

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	SchemaVersion = 1

	ModeSidecar = "sidecar"

	PlanningModeRunManifest   = "run_manifest"
	PlanningModePlanningFiles = "planning_files"
	PlanningModeHybrid        = "hybrid"

	ChecklistPending    = "pending"
	ChecklistInProgress = "in_progress"
	ChecklistBlocked    = "blocked"
	ChecklistDone       = "done"
	ChecklistDropped    = "dropped"

	CloseStatusOpen     = "open"
	CloseStatusReview   = "review"
	CloseStatusDone     = "done"
	CloseStatusBlocked  = "blocked"
	CloseStatusCanceled = "canceled"
)

var (
	validChecklistStatuses = []string{
		ChecklistPending,
		ChecklistInProgress,
		ChecklistBlocked,
		ChecklistDone,
		ChecklistDropped,
	}
	validPlanningModes = []string{
		PlanningModeRunManifest,
		PlanningModePlanningFiles,
		PlanningModeHybrid,
	}
	validCloseStatuses = []string{
		CloseStatusOpen,
		CloseStatusReview,
		CloseStatusDone,
		CloseStatusBlocked,
		CloseStatusCanceled,
	}
)

type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	TaskID        string          `json:"task_id"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	Mode          string          `json:"mode"`
	PlanningMode  string          `json:"planning_mode"`
	Checklist     []ChecklistItem `json:"checklist"`
	Attempts      []Attempt       `json:"attempts"`
	Audit         []AuditItem     `json:"audit"`
	Close         CloseState      `json:"close"`
}

type ChecklistItem struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Notes  []string `json:"notes"`
}

type Attempt struct {
	ID        string       `json:"id"`
	StartedAt string       `json:"started_at"`
	EndedAt   string       `json:"ended_at,omitempty"`
	Summary   string       `json:"summary,omitempty"`
	Receipts  []ReceiptRef `json:"receipts"`
}

type ReceiptRef struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type AuditItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	ReceiptPath string `json:"receipt_path,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type CloseState struct {
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
	ClosedAt  string `json:"closed_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ChecklistSummary struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Done       int `json:"done"`
	Dropped    int `json:"dropped"`
}

func NewManifest(taskID string, now time.Time) (Manifest, error) {
	taskID = strings.TrimSpace(taskID)
	if err := adapter.ValidateTaskID(taskID); err != nil {
		return Manifest{}, fmt.Errorf("invalid task id: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339)
	return Manifest{
		SchemaVersion: SchemaVersion,
		TaskID:        taskID,
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
		Mode:          ModeSidecar,
		PlanningMode:  PlanningModeRunManifest,
		Checklist:     []ChecklistItem{},
		Attempts:      []Attempt{},
		Audit:         []AuditItem{},
		Close: CloseState{
			Status:    CloseStatusOpen,
			UpdatedAt: timestamp,
		},
	}, nil
}

func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported run manifest schema_version %d", manifest.SchemaVersion)
	}
	if err := adapter.ValidateTaskID(manifest.TaskID); err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	if strings.TrimSpace(manifest.CreatedAt) == "" {
		return fmt.Errorf("created_at is required")
	}
	if strings.TrimSpace(manifest.UpdatedAt) == "" {
		return fmt.Errorf("updated_at is required")
	}
	if manifest.Mode != ModeSidecar {
		return fmt.Errorf("mode must be %q", ModeSidecar)
	}
	if !slices.Contains(validPlanningModes, manifest.PlanningMode) {
		return fmt.Errorf("planning_mode must be one of: %s", strings.Join(validPlanningModes, ", "))
	}
	if err := ValidateCloseStatus(manifest.Close.Status); err != nil {
		return fmt.Errorf("close.status: %w", err)
	}
	seen := map[string]bool{}
	for index, item := range manifest.Checklist {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("checklist[%d].id is required", index)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate checklist id %q", item.ID)
		}
		seen[item.ID] = true
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("checklist[%d].title is required", index)
		}
		if err := ValidateChecklistStatus(item.Status); err != nil {
			return fmt.Errorf("checklist[%d].status: %w", index, err)
		}
	}
	for index, item := range manifest.Audit {
		if strings.TrimSpace(item.Kind) == "" {
			return fmt.Errorf("audit[%d].kind is required", index)
		}
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("audit[%d].name is required", index)
		}
		if strings.TrimSpace(item.Status) == "" {
			return fmt.Errorf("audit[%d].status is required", index)
		}
		if strings.TrimSpace(item.CreatedAt) == "" {
			return fmt.Errorf("audit[%d].created_at is required", index)
		}
	}
	return nil
}

func ValidateChecklistStatus(status string) error {
	status = NormalizeChecklistStatus(status)
	if slices.Contains(validChecklistStatuses, status) {
		return nil
	}
	return fmt.Errorf("must be one of: %s; complete/completed are accepted aliases for done", strings.Join(validChecklistStatuses, ", "))
}

func ValidateCloseStatus(status string) error {
	status = NormalizeCloseStatus(status)
	if slices.Contains(validCloseStatuses, status) {
		return nil
	}
	return fmt.Errorf("must be one of: %s; complete/completed are accepted aliases for done", strings.Join(validCloseStatuses, ", "))
}

func NormalizeChecklistStatus(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case "complete", "completed":
		return ChecklistDone
	default:
		return status
	}
}

func NormalizeCloseStatus(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case "complete", "completed":
		return CloseStatusDone
	default:
		return status
	}
}

func (manifest *Manifest) AddChecklistItem(id string, title string, now time.Time) error {
	id = strings.TrimSpace(id)
	title = strings.TrimSpace(title)
	if id == "" {
		return fmt.Errorf("checklist item id is required")
	}
	if title == "" {
		return fmt.Errorf("checklist item title is required")
	}
	if findChecklistItem(manifest.Checklist, id) >= 0 {
		return fmt.Errorf("checklist item %q already exists", id)
	}
	manifest.Checklist = append(manifest.Checklist, ChecklistItem{
		ID:     id,
		Title:  title,
		Status: ChecklistPending,
		Notes:  []string{},
	})
	manifest.touch(now)
	return manifest.Validate()
}

func (manifest *Manifest) SetChecklistItem(id string, status string, note string, now time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("checklist item id is required")
	}
	if err := ValidateChecklistStatus(status); err != nil {
		return err
	}
	index := findChecklistItem(manifest.Checklist, id)
	if index < 0 {
		return fmt.Errorf("checklist item %q not found", id)
	}
	manifest.Checklist[index].Status = NormalizeChecklistStatus(status)
	if value := strings.TrimSpace(note); value != "" {
		manifest.Checklist[index].Notes = append(manifest.Checklist[index].Notes, value)
	}
	manifest.touch(now)
	return manifest.Validate()
}

func (manifest *Manifest) AddAuditItem(item AuditItem, now time.Time) error {
	item.Kind = strings.TrimSpace(item.Kind)
	item.Name = strings.TrimSpace(item.Name)
	item.Status = strings.TrimSpace(item.Status)
	item.Summary = strings.TrimSpace(item.Summary)
	item.ReceiptPath = filepath.ToSlash(strings.TrimSpace(item.ReceiptPath))
	if item.Kind == "" {
		return fmt.Errorf("audit kind is required")
	}
	if item.Name == "" {
		return fmt.Errorf("audit name is required")
	}
	if item.Status == "" {
		return fmt.Errorf("audit status is required")
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now.UTC().Format(time.RFC3339)
	}
	manifest.Audit = append(manifest.Audit, item)
	manifest.touch(now)
	return manifest.Validate()
}

func (manifest *Manifest) CloseRun(status string, summary string, now time.Time) error {
	status = NormalizeCloseStatus(status)
	if err := ValidateCloseStatus(status); err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	manifest.Close.Status = status
	manifest.Close.Summary = strings.TrimSpace(summary)
	manifest.Close.UpdatedAt = timestamp
	if status == CloseStatusOpen {
		manifest.Close.ClosedAt = ""
	} else {
		manifest.Close.ClosedAt = timestamp
	}
	manifest.touch(now)
	return manifest.Validate()
}

func (manifest Manifest) ChecklistSummary() ChecklistSummary {
	summary := ChecklistSummary{}
	for _, item := range manifest.Checklist {
		switch NormalizeChecklistStatus(item.Status) {
		case ChecklistPending:
			summary.Pending++
		case ChecklistInProgress:
			summary.InProgress++
		case ChecklistBlocked:
			summary.Blocked++
		case ChecklistDone:
			summary.Done++
		case ChecklistDropped:
			summary.Dropped++
		}
	}
	return summary
}

func (manifest *Manifest) touch(now time.Time) {
	manifest.UpdatedAt = now.UTC().Format(time.RFC3339)
	manifest.Close.UpdatedAt = manifest.UpdatedAt
}

func findChecklistItem(items []ChecklistItem, id string) int {
	for index, item := range items {
		if item.ID == id {
			return index
		}
	}
	return -1
}
