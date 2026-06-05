package surface

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type CommandProbe struct {
	Command     string `json:"command"`
	Usage       string `json:"usage"`
	JSONSupport bool   `json:"json_support"`
}

type CapabilityReceipt struct {
	SchemaVersion int            `json:"schema_version"`
	AntonVersion  string         `json:"anton_version"`
	Commands      []CommandProbe `json:"commands"`
	ConfigFields  []string       `json:"config_fields"`
	SchemaHashes  SchemaHashes   `json:"schema_hashes"`
}

type SchemaHashes struct {
	CommandSurface string `json:"command_surface"`
	ConfigFields   string `json:"config_fields"`
}

func CommandSurface() []CommandProbe {
	return []CommandProbe{
		{Command: "doctor", Usage: "anton doctor [--json]", JSONSupport: true},
		{Command: "context", Usage: "anton context [--json|--explain]", JSONSupport: true},
		{Command: "preflight", Usage: "anton preflight --profile <investigation|implementation> [--json]", JSONSupport: true},
		{Command: "task-state", Usage: "anton task-state <init|pulse|check|env|service|freshness|sync-card|close|reopen|retarget|import> [--freshness] [--strict] [--json]", JSONSupport: true},
		{Command: "task", Usage: "anton task <resolve|list> [--check] [--json]", JSONSupport: true},
		{Command: "run", Usage: "anton run <init|status|task|audit|close>; run audit events [--json]", JSONSupport: true},
		{Command: "handoff", Usage: "anton handoff <build|persist-results> [--json]", JSONSupport: true},
		{Command: "threads", Usage: "anton threads <doctor|recent|insights|brief|recipe> [--json]", JSONSupport: true},
		{Command: "adopt", Usage: "anton adopt plan [--json]", JSONSupport: true},
		{Command: "memory", Usage: "anton memory <status|update> [--json]", JSONSupport: true},
		{Command: "history", Usage: "anton history <show|sync> [--json]", JSONSupport: true},
		{Command: "gates", Usage: "anton gates <list|check|run> [--json]", JSONSupport: true},
		{Command: "check", Usage: "anton check <run|repair-plan> [--json]", JSONSupport: true},
		{Command: "entrypoint", Usage: "anton entrypoint check [--json]", JSONSupport: true},
		{Command: "workspace", Usage: "anton workspace <inspect|check|refs|list|doctor|cleanup-plan|worktrees> [--json]", JSONSupport: true},
		{Command: "migrate", Usage: "anton migrate <plan|readiness|project-progress> [--json]", JSONSupport: true},
		{Command: "version", Usage: "anton version [--json]", JSONSupport: true},
	}
}

func ConfigFieldPaths() []string {
	return []string{
		"version",
		"entrypoint.path",
		"tasks.root",
		"tasks.layout",
		"tasks.topic_layer",
		"tasks.status_schema",
		"tasks.card_sync",
		"tasks.planning_mode",
		"run.enabled",
		"run.manifest",
		"run.receipts_dir",
		"threads.default_project_strategy",
		"threads.workspace_roots",
		"gates",
		"gate_profiles",
		"roots.state",
		"roots.memory",
		"roots.artifacts",
		"roots.archive",
		"roots.views",
		"roots.policy_registry",
		"migrate.target_schema.version",
		"migrate.target_schema.locked",
		"migrate.target_schema.reason",
		"migrate.default_target",
		"extensions.history.work_record_roots",
	}
}

func GlobalUsage(version string) string {
	lines := []string{
		fmt.Sprintf("Anton %s is a reusable harness CLI.", version),
		"",
		"Usage:",
		"  anton [--json] <command> [...]",
	}
	for _, probe := range CommandSurface() {
		lines = append(lines, "  "+probe.Usage)
	}
	lines = append(lines,
		"",
		"Flags:",
		"  --help",
		"  --version",
		"",
	)
	return strings.Join(lines, "\n")
}

func Capabilities(version string) CapabilityReceipt {
	commands := CommandSurface()
	fields := ConfigFieldPaths()
	return CapabilityReceipt{
		SchemaVersion: 1,
		AntonVersion:  version,
		Commands:      commands,
		ConfigFields:  fields,
		SchemaHashes: SchemaHashes{
			CommandSurface: hashCommandSurface(commands),
			ConfigFields:   hashStrings(fields),
		},
	}
}

func hashCommandSurface(commands []CommandProbe) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, fmt.Sprintf("%s|%s|%t", command.Command, command.Usage, command.JSONSupport))
	}
	return hashStrings(lines)
}

func hashStrings(values []string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:])
}
