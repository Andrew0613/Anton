package gates

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	levelError   = "error"
	levelWarning = "warning"
)

var allowedTypes = map[string]bool{
	"command":  true,
	"manual":   true,
	"external": true,
}

var allowedRequiredFor = map[string]bool{
	"blocked":        true,
	"closeout":       true,
	"done":           true,
	"handoff":        true,
	"implementation": true,
	"partial":        true,
	"review":         true,
}

type Source struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Loaded bool   `json:"loaded"`
}

type Gate struct {
	Name        string           `json:"name" yaml:"name"`
	Type        string           `json:"type" yaml:"type"`
	RequiredFor []string         `json:"required_for,omitempty" yaml:"required_for"`
	Description string           `json:"description,omitempty" yaml:"description"`
	Command     *CommandMetadata `json:"command,omitempty" yaml:"command"`
	Timeout     *TimeoutMetadata `json:"timeout,omitempty" yaml:"timeout"`
	Destructive bool             `json:"destructive,omitempty" yaml:"destructive"`
}

type CommandMetadata struct {
	Argv             []string `json:"argv,omitempty" yaml:"argv"`
	WorkingDirectory string   `json:"working_directory,omitempty" yaml:"working_directory"`
}

type TimeoutMetadata struct {
	Seconds int `json:"seconds,omitempty" yaml:"seconds"`
}

type Finding struct {
	Level       string `json:"level"`
	Code        string `json:"code"`
	Gate        string `json:"gate,omitempty"`
	Field       string `json:"field,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type Summary struct {
	Declared    int            `json:"declared"`
	Required    int            `json:"required"`
	Advisory    int            `json:"advisory"`
	RequiredFor map[string]int `json:"required_for,omitempty"`
	Errors      int            `json:"errors"`
	Warnings    int            `json:"warnings"`
	Unsafe      int            `json:"unsafe"`
	Destructive int            `json:"destructive"`
}

type Set struct {
	Source   Source             `json:"source"`
	Gates    []Gate             `json:"gates"`
	Profiles map[string]Profile `json:"profiles,omitempty"`
	Findings []Finding          `json:"findings"`
	Summary  Summary            `json:"summary"`
}

type Profile struct {
	Required []string `json:"required,omitempty" yaml:"required"`
}

type rawConfig struct {
	Version      int                `yaml:"version"`
	Entrypoint   map[string]any     `yaml:"entrypoint"`
	Tasks        map[string]any     `yaml:"tasks"`
	Run          map[string]any     `yaml:"run"`
	Threads      map[string]any     `yaml:"threads"`
	Gates        []Gate             `yaml:"gates"`
	GateProfiles map[string]Profile `yaml:"gate_profiles"`
	Extensions   map[string]any     `yaml:"extensions"`
}

func Parse(data []byte, source Source) (Set, error) {
	config := rawConfig{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Set{}, fmt.Errorf("parse %s: %w", source.Path, err)
	}

	var extraDoc any
	err := decoder.Decode(&extraDoc)
	if err == nil {
		return Set{}, fmt.Errorf("parse %s: multiple YAML documents are not supported", source.Path)
	}
	if err != io.EOF {
		return Set{}, fmt.Errorf("parse %s: %w", source.Path, err)
	}

	if config.Version != 1 {
		return Set{}, fmt.Errorf("unsupported anton config version %d", config.Version)
	}

	set := Set{
		Source:   source,
		Gates:    normalizeGates(config.Gates),
		Profiles: normalizeProfiles(config.GateProfiles),
	}
	set.Findings = validateGates(set.Gates)
	set.Summary = summarize(set.Gates, set.Findings)
	return set, nil
}

func LoadFile(path string, sourceName string) (Set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Set{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(content, Source{
		Path:   filepath.Clean(path),
		Source: sourceName,
		Loaded: true,
	})
}

func EmptySet(path string) Set {
	set := Set{
		Source: Source{
			Path:   filepath.Clean(path),
			Source: "built-in defaults",
			Loaded: false,
		},
		Gates:    []Gate{},
		Findings: []Finding{},
	}
	set.Summary = summarize(set.Gates, set.Findings)
	return set
}

func (set Set) OK() bool {
	return set.Summary.Errors == 0
}

func normalizeGates(gates []Gate) []Gate {
	if gates == nil {
		return []Gate{}
	}
	normalized := make([]Gate, 0, len(gates))
	for _, gate := range gates {
		gate.Name = strings.TrimSpace(gate.Name)
		gate.Type = strings.TrimSpace(gate.Type)
		gate.Description = strings.TrimSpace(gate.Description)
		gate.RequiredFor = normalizeStringList(gate.RequiredFor)
		if gate.Command != nil {
			gate.Command.WorkingDirectory = strings.TrimSpace(gate.Command.WorkingDirectory)
			gate.Command.Argv = normalizeStringList(gate.Command.Argv)
		}
		normalized = append(normalized, gate)
	}
	return normalized
}

func normalizeProfiles(profiles map[string]Profile) map[string]Profile {
	if len(profiles) == 0 {
		return nil
	}
	normalized := map[string]Profile{}
	for name, profile := range profiles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		profile.Required = normalizeStringList(profile.Required)
		normalized[name] = profile
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func validateGates(gates []Gate) []Finding {
	findings := []Finding{}
	seen := map[string]bool{}

	for index, gate := range gates {
		label := gate.Name
		if label == "" {
			label = fmt.Sprintf("gates[%d]", index)
		}

		if gate.Name == "" {
			findings = append(findings, errorFinding(label, "name", "missing-gate-name", "gate name must not be empty", "give each gate a stable name"))
		} else if seen[gate.Name] {
			findings = append(findings, errorFinding(label, "name", "duplicate-gate-name", fmt.Sprintf("gate %q is declared more than once", gate.Name), "rename or remove the duplicate gate"))
		}
		seen[gate.Name] = true

		if gate.Type == "" {
			findings = append(findings, errorFinding(label, "type", "missing-gate-type", "gate type must not be empty", "set type to command, manual, or external"))
		} else if !allowedTypes[gate.Type] {
			findings = append(findings, errorFinding(label, "type", "invalid-gate-type", fmt.Sprintf("gate type %q must be one of: command, manual, external", gate.Type), "set type to command, manual, or external"))
		}

		for _, target := range gate.RequiredFor {
			if !allowedRequiredFor[target] {
				findings = append(findings, errorFinding(label, "required_for", "invalid-required-for", fmt.Sprintf("required_for value %q must be one of: blocked, closeout, done, handoff, implementation, partial, review", target), "use a supported gate profile or leave required_for empty for advisory gates"))
			}
		}

		if gate.Type == "command" && isRequired(gate) && (gate.Command == nil || len(gate.Command.Argv) == 0) {
			findings = append(findings, warningFinding(label, "command.argv", "missing-command-metadata", "command gates need command.argv before they can be executed by gates run", "declare command.argv, or keep the gate manual/external"))
		}

		if gate.Command != nil {
			if len(gate.Command.Argv) == 0 && !isRequired(gate) {
				findings = append(findings, warningFinding(label, "command.argv", "missing-optional-command-argv", "optional command gate has no command.argv metadata", "declare command.argv for auditability, or remove empty command metadata"))
			}
			for argIndex, arg := range gate.Command.Argv {
				if arg == "" {
					findings = append(findings, errorFinding(label, fmt.Sprintf("command.argv[%d]", argIndex), "empty-command-argument", "command.argv entries must not be empty", "remove the empty argv entry"))
					continue
				}
				if looksShellLike(arg) {
					findings = append(findings, warningFinding(label, fmt.Sprintf("command.argv[%d]", argIndex), "unsafe-command-content", "command metadata contains shell-like content; gates run blocks shell execution", "prefer argv-style metadata without shell operators"))
				}
			}
		}

		if gate.Timeout != nil && gate.Timeout.Seconds <= 0 {
			findings = append(findings, errorFinding(label, "timeout.seconds", "invalid-timeout", "timeout.seconds must be greater than zero when declared", "set a positive timeout or remove timeout metadata"))
		}

		if gate.Destructive {
			findings = append(findings, warningFinding(label, "destructive", "destructive-gate-blocked", "destructive gate metadata is visible but gates run blocks it by default", "keep destructive gates advisory until a separate execution opt-in policy lands"))
		}
	}

	return findings
}

func summarize(gates []Gate, findings []Finding) Summary {
	summary := Summary{
		Declared:    len(gates),
		RequiredFor: map[string]int{},
	}
	for _, gate := range gates {
		if isRequired(gate) {
			summary.Required++
			for _, target := range gate.RequiredFor {
				summary.RequiredFor[target]++
			}
		} else {
			summary.Advisory++
		}
		if gate.Destructive {
			summary.Destructive++
		}
	}
	for _, finding := range findings {
		switch finding.Level {
		case levelError:
			summary.Errors++
		case levelWarning:
			summary.Warnings++
		}
		if finding.Code == "unsafe-command-content" {
			summary.Unsafe++
		}
	}
	return summary
}

func isRequired(gate Gate) bool {
	return len(gate.RequiredFor) > 0
}

func normalizeStringList(values []string) []string {
	if values == nil {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func looksShellLike(value string) bool {
	return strings.ContainsAny(value, "\n\r;&|<>`$")
}

func errorFinding(gate string, field string, code string, message string, remediation string) Finding {
	return Finding{
		Level:       levelError,
		Code:        code,
		Gate:        gate,
		Field:       field,
		Message:     message,
		Remediation: remediation,
	}
}

func warningFinding(gate string, field string, code string, message string, remediation string) Finding {
	return Finding{
		Level:       levelWarning,
		Code:        code,
		Gate:        gate,
		Field:       field,
		Message:     message,
		Remediation: remediation,
	}
}
