package policy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	RuleID          string    `json:"rule_id" yaml:"rule_id"`
	Owner           string    `json:"owner" yaml:"owner"`
	Category        string    `json:"category" yaml:"category"`
	Severity        string    `json:"severity" yaml:"severity"`
	Tier            string    `json:"tier,omitempty" yaml:"tier"`
	CanonicalSource string    `json:"canonical_source,omitempty" yaml:"canonical_source"`
	PlannedSurface  string    `json:"planned_anton_surface,omitempty" yaml:"planned_anton_surface"`
	Summary         string    `json:"summary,omitempty" yaml:"summary"`
	Blocking        bool      `json:"blocking,omitempty" yaml:"blocking"`
	ArchiveOnly     bool      `json:"archive_only,omitempty" yaml:"archive_only"`
	Autofix         bool      `json:"autofix,omitempty" yaml:"autofix"`
	SafeCommand     string    `json:"safe_command,omitempty" yaml:"safe_command"`
	Check           CheckSpec `json:"check" yaml:"check"`
}

type CheckSpec struct {
	Kind        string   `json:"kind" yaml:"kind"`
	Path        string   `json:"path,omitempty" yaml:"path"`
	PathPattern string   `json:"path_pattern,omitempty" yaml:"path_pattern"`
	Contains    string   `json:"contains,omitempty" yaml:"contains"`
	Tokens      []string `json:"tokens,omitempty" yaml:"tokens"`
	Field       string   `json:"field,omitempty" yaml:"field"`
	Equals      string   `json:"equals,omitempty" yaml:"equals"`
	Sections    []string `json:"sections,omitempty" yaml:"sections"`
}

type Issue struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	File       string `json:"file,omitempty"`
	Message    string `json:"message"`
	RepairHint string `json:"repair_hint,omitempty"`
}

type Registry struct {
	Root         string  `json:"root"`
	SourceFile   string  `json:"source_file,omitempty"`
	Rules        []Rule  `json:"rules"`
	Issues       []Issue `json:"issues,omitempty"`
	Loaded       bool    `json:"loaded"`
	ConfigPath   string  `json:"config_path"`
	ConfigSource string  `json:"config_source"`
}

type rawRegistry struct {
	SchemaVersion int    `yaml:"schema_version"`
	Status        string `yaml:"status"`
	Authority     string `yaml:"authority"`
	Rules         []Rule `yaml:"rules"`
	Entries       []Rule `yaml:"entries"`
}

func Load(resolved adapter.Resolved) (Registry, error) {
	base := resolved.Context.WorkingDirectory
	if resolved.Context.RepositoryRoot != "" {
		base = resolved.Context.RepositoryRoot
	}
	root := filepath.Clean(filepath.Join(base, resolved.Config.PolicyRegistryRoot()))
	registry := Registry{
		Root:         root,
		Rules:        []Rule{},
		Issues:       []Issue{},
		ConfigPath:   resolved.Config.Path,
		ConfigSource: resolved.Config.Source(),
	}
	candidates := []string{
		filepath.Join(root, "checks.yaml"),
		filepath.Join(root, "rules.yaml"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		loaded, loadErr := loadRegistryFile(candidate)
		registry.SourceFile = candidate
		registry.Loaded = true
		registry.Rules = loaded.Rules
		registry.Issues = append(registry.Issues, loaded.Issues...)
		if loadErr != nil {
			registry.Issues = append(registry.Issues, Issue{
				Level:      "error",
				Code:       "policy-registry-parse-failed",
				File:       candidate,
				Message:    loadErr.Error(),
				RepairHint: "fix YAML syntax and required rule fields",
			})
		}
		return registry, nil
	}

	registry.Issues = append(registry.Issues, Issue{
		Level:      "warning",
		Code:       "policy-registry-missing",
		File:       root,
		Message:    "no policy registry file found (expected checks.yaml or rules.yaml)",
		RepairHint: "add docs/agent-workflow/registries/checks.yaml to enable rule-based checks",
	})
	return registry, nil
}

func loadRegistryFile(path string) (Registry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	raw := rawRegistry{}
	if err := decoder.Decode(&raw); err != nil {
		return Registry{}, err
	}
	var extra any
	err = decoder.Decode(&extra)
	if err == nil {
		return Registry{}, fmt.Errorf("multiple YAML documents are not supported")
	}
	if err != io.EOF {
		return Registry{}, err
	}
	registry := Registry{
		Root:       filepath.Dir(path),
		SourceFile: path,
		Rules:      normalizeRules(append(raw.Rules, raw.Entries...)),
		Issues:     []Issue{},
		Loaded:     true,
	}
	registry.Issues = append(registry.Issues, validateRules(registry.Rules, path)...)
	return registry, nil
}

func normalizeRules(rules []Rule) []Rule {
	if rules == nil {
		return []Rule{}
	}
	result := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		rule.RuleID = strings.TrimSpace(rule.RuleID)
		rule.Owner = strings.TrimSpace(rule.Owner)
		rule.Category = strings.TrimSpace(rule.Category)
		rule.Tier = strings.TrimSpace(rule.Tier)
		rule.Severity = strings.TrimSpace(strings.ToLower(rule.Severity))
		rule.Severity = normalizeSeverity(rule.Severity)
		if rule.Category == "" {
			rule.Category = fallback(rule.Tier, "policy")
		}
		rule.CanonicalSource = strings.TrimSpace(rule.CanonicalSource)
		rule.PlannedSurface = strings.TrimSpace(rule.PlannedSurface)
		rule.Summary = strings.TrimSpace(rule.Summary)
		rule.SafeCommand = strings.TrimSpace(rule.SafeCommand)
		rule.Check.Kind = strings.TrimSpace(rule.Check.Kind)
		rule.Check.Path = strings.TrimSpace(rule.Check.Path)
		rule.Check.PathPattern = strings.TrimSpace(rule.Check.PathPattern)
		rule.Check.Contains = strings.TrimSpace(rule.Check.Contains)
		for i, t := range rule.Check.Tokens {
			rule.Check.Tokens[i] = strings.TrimSpace(t)
		}
		rule.Check.Field = strings.TrimSpace(rule.Check.Field)
		rule.Check.Equals = strings.TrimSpace(rule.Check.Equals)
		for i, s := range rule.Check.Sections {
			rule.Check.Sections[i] = strings.TrimSpace(s)
		}
		result = append(result, rule)
	}
	return result
}

func validateRules(rules []Rule, source string) []Issue {
	issues := []Issue{}
	seen := map[string]bool{}
	for index, rule := range rules {
		label := rule.RuleID
		if label == "" {
			label = fmt.Sprintf("rules[%d]", index)
		}
		if rule.RuleID == "" {
			issues = append(issues, Issue{
				Level:      "error",
				Code:       "policy-rule-id-missing",
				File:       source,
				Message:    fmt.Sprintf("%s: rule_id is required", label),
				RepairHint: "set a stable rule_id",
			})
		} else if seen[rule.RuleID] {
			issues = append(issues, Issue{
				Level:      "error",
				Code:       "policy-rule-id-duplicate",
				File:       source,
				Message:    fmt.Sprintf("%s: duplicate rule_id", label),
				RepairHint: "deduplicate rule_id entries",
			})
		}
		seen[rule.RuleID] = true
		if rule.Owner == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-owner-missing", File: source, Message: fmt.Sprintf("%s: owner is required", label), RepairHint: "set owner"})
		}
		if rule.Severity != "warning" && rule.Severity != "error" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-severity-invalid", File: source, Message: fmt.Sprintf("%s: severity must be warning or error", label), RepairHint: "set severity to warning or error"})
		}
		if rule.Check.Kind == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-kind-missing", File: source, Message: fmt.Sprintf("%s: check.kind is required", label), RepairHint: "set check.kind"})
		} else {
			issues = append(issues, validateCheckSpec(rule, label, source)...)
		}
	}
	return issues
}

func validateCheckSpec(rule Rule, label string, source string) []Issue {
	issues := []Issue{}
	requirePath := func() {
		if rule.Check.Path == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-path-missing", File: source, Message: fmt.Sprintf("%s: check.path is required for %s", label, rule.Check.Kind), RepairHint: "set check.path"})
		}
	}
	requireContains := func() {
		if rule.Check.Contains == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-contains-missing", File: source, Message: fmt.Sprintf("%s: check.contains is required for %s", label, rule.Check.Kind), RepairHint: "set check.contains"})
		}
	}
	requireField := func() {
		if rule.Check.Field == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-field-missing", File: source, Message: fmt.Sprintf("%s: check.field is required for %s", label, rule.Check.Kind), RepairHint: "set check.field"})
		}
	}
	requireEquals := func() {
		if rule.Check.Equals == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-equals-missing", File: source, Message: fmt.Sprintf("%s: check.equals is required for %s", label, rule.Check.Kind), RepairHint: "set check.equals"})
		}
	}
	switch rule.Check.Kind {
	case "path_exists", "path_missing":
		requirePath()
	case "file_contains":
		requirePath()
		requireContains()
	case "yaml_field_equals", "yaml_all_fields_equal", "frontmatter_field_equals":
		requirePath()
		requireField()
		requireEquals()
	case "frontmatter_field_present":
		requirePath()
		requireField()
	case "file_contains_all":
		requirePath()
		if len(rule.Check.Tokens) == 0 {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-tokens-missing", File: source, Message: fmt.Sprintf("%s: check.tokens is required for %s", label, rule.Check.Kind), RepairHint: "set check.tokens"})
		}
	case "markdown_has_sections":
		if rule.Check.Path == "" && rule.Check.PathPattern == "" {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-path-missing", File: source, Message: fmt.Sprintf("%s: check.path or check.path_pattern is required for %s", label, rule.Check.Kind), RepairHint: "set check.path or check.path_pattern"})
		}
		if len(rule.Check.Sections) == 0 {
			issues = append(issues, Issue{Level: "error", Code: "policy-rule-check-sections-missing", File: source, Message: fmt.Sprintf("%s: check.sections is required for %s", label, rule.Check.Kind), RepairHint: "set check.sections"})
		}
	case "state_dual_read_parity", "state_projection_source_integrity":
	default:
	}
	return issues
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error", "critical", "high", "blocking":
		return "error"
	default:
		return "warning"
	}
}

func fallback(value string, fallbackValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallbackValue
	}
	return value
}
