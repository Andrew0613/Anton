package adapter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var taskIDSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateTaskID enforces a path-safe task id slug.
func ValidateTaskID(taskID string) error {
	value := strings.TrimSpace(taskID)
	if value == "" {
		return fmt.Errorf("task id cannot be empty")
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("task id must not be an absolute path")
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return fmt.Errorf("task id must not contain path separators")
	}
	if strings.Contains(value, "..") {
		return fmt.Errorf("task id must not contain '..'")
	}
	if !taskIDSlugPattern.MatchString(value) {
		return fmt.Errorf("task id must match %q", taskIDSlugPattern.String())
	}
	return nil
}
