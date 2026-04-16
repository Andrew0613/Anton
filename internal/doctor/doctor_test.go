package doctor

import (
	"testing"

	"github.com/Andrew0613/Anton/internal/adapter"
)

func TestSummarizeChecks(t *testing.T) {
	result := summarizeChecks([]check{
		{Name: "a", Status: statusOK},
		{Name: "b", Status: statusDegraded},
		{Name: "c", Status: statusBlocked},
	})

	if result.Status != statusBlocked {
		t.Fatalf("summary status = %q, want blocked", result.Status)
	}
	if result.OKCount != 1 || result.DegradedCount != 1 || result.BlockedCount != 1 {
		t.Fatalf("unexpected counts: %+v", result)
	}
}

func TestCheckAntonConfigReportsLoadedFile(t *testing.T) {
	result := checkAntonConfig(adapter.Config{
		Path:   "/tmp/repo/anton.yaml",
		Loaded: true,
	})

	if result.Status != statusOK {
		t.Fatalf("status = %q, want %q", result.Status, statusOK)
	}
}

func TestCheckAntonConfigReportsMissingFile(t *testing.T) {
	result := checkAntonConfig(adapter.Config{
		Path: "/tmp/repo/anton.yaml",
	})

	if result.Status != statusDegraded {
		t.Fatalf("status = %q, want %q", result.Status, statusDegraded)
	}
	if result.Hint == "" {
		t.Fatalf("expected hint for missing anton.yaml")
	}
}
