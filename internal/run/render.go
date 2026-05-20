package run

import "fmt"

func RenderSummary(manifest Manifest) string {
	summary := manifest.ChecklistSummary()
	return fmt.Sprintf(
		"run manifest: task=%s close=%s checklist[pending=%d in_progress=%d blocked=%d done=%d dropped=%d] audit=%d",
		manifest.TaskID,
		manifest.Close.Status,
		summary.Pending,
		summary.InProgress,
		summary.Blocked,
		summary.Done,
		summary.Dropped,
		len(manifest.Audit),
	)
}
