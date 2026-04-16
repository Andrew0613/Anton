package threads

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Andrew0613/Anton/internal/adapter"
)

func TestScopeWarningRequiresProject(t *testing.T) {
	if warning := scopeWarning("Anton"); warning != "" {
		t.Fatalf("scopeWarning returned %q for project-scoped request", warning)
	}
	if warning := scopeWarning(""); warning == "" {
		t.Fatalf("scopeWarning should warn when project scope is missing")
	}
}

func TestFindOnPathReturnsExecutableCandidate(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "codex-threads")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	got := findOnPath("codex-threads", root)
	if got != binary {
		t.Fatalf("findOnPath returned %q, want %q", got, binary)
	}
}

func TestParseOptionsRejectsUnexpectedArguments(t *testing.T) {
	if _, err := parseOptions([]string{"--bad-flag"}, true, 20); err == nil {
		t.Fatalf("parseOptions should reject unexpected arguments")
	}
}

func TestThreadsUseAdapterProjectResolution(t *testing.T) {
	context := adapter.Context{
		WorkingDirectory: "/tmp/Anton",
		RepositoryRoot:   "/tmp/Anton",
	}

	project := adapter.Default{}.ResolveThreadsProject(context, nil, "")
	if project.Name != "Anton" || project.Source != "repo-root" {
		t.Fatalf("adapter project = %#v", project)
	}
}
