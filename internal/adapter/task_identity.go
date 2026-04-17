package adapter

import (
	"path/filepath"
	"slices"
	"strings"
)

type TaskIdentitySignal struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

type TaskIdentity struct {
	Resolved       string               `json:"resolved,omitempty"`
	Signals        []TaskIdentitySignal `json:"signals,omitempty"`
	Conflict       bool                 `json:"conflict"`
	ConflictValues []string             `json:"conflict_values,omitempty"`
	BundleRoot     string               `json:"bundle_root,omitempty"`
}

func ResolveTaskIdentity(context Context, config Config, environ []string) TaskIdentity {
	effective := config
	if effective.Version == 0 {
		effective = defaultConfig()
	}
	definition := Default{Config: effective}
	tasksRoot := definition.tasksRoot(context)

	signals := make([]TaskIdentitySignal, 0, 3)
	if values := envMap(environ); strings.TrimSpace(values["ANTON_TASK_ID"]) != "" {
		signals = append(signals, TaskIdentitySignal{
			Source: "env",
			Value:  strings.TrimSpace(values["ANTON_TASK_ID"]),
		})
	}
	if matches := genericTaskBranchPattern.FindStringSubmatch(context.GitBranch); len(matches) == 2 {
		signals = append(signals, TaskIdentitySignal{
			Source: "branch",
			Value:  strings.TrimSpace(matches[1]),
		})
	}

	bundleRoot := currentTaskBundleRoot(context.WorkingDirectory, tasksRoot)
	if strings.TrimSpace(bundleRoot) != "" {
		signals = append(signals, TaskIdentitySignal{
			Source: "bundle",
			Value:  filepath.Base(bundleRoot),
		})
	}

	values := make([]string, 0, len(signals))
	for _, signal := range signals {
		if strings.TrimSpace(signal.Value) == "" {
			continue
		}
		if !slices.Contains(values, signal.Value) {
			values = append(values, signal.Value)
		}
	}

	identity := TaskIdentity{
		Signals:    signals,
		BundleRoot: bundleRoot,
	}
	switch len(values) {
	case 0:
		return identity
	case 1:
		identity.Resolved = values[0]
		return identity
	default:
		identity.Conflict = true
		identity.ConflictValues = values
		return identity
	}
}
