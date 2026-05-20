package gates

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Andrew0613/Anton/internal/adapter"
)

type options struct {
	JSON       bool
	ConfigPath string
}

type runOptions struct {
	JSON       bool
	ConfigPath string
	GateName   string
	Profile    string
	DryRun     bool
	AttachRun  bool
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *Set          `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "list":
		return runList(args[1:], stdout, stderr, environ)
	case "check":
		return runCheck(args[1:], stdout, stderr, environ)
	case "run":
		opts, err := parseRunOptions(args[1:])
		if err != nil {
			return writeError("gates run", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
		}
		return runDeclaredGates(opts, stdout, stderr, environ)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown gates command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runList(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("gates list", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	set, err := loadSet(opts, environ)
	if err != nil {
		return writeError("gates list", "gates-list-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	return writeResponse("gates list", set, opts.JSON, stdout, set.OK())
}

func runCheck(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("gates check", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	set, err := loadSet(opts, environ)
	if err != nil {
		return writeError("gates check", "gates-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}
	return writeResponse("gates check", set, opts.JSON, stdout, set.OK())
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		if arg == "--json" {
			opts.JSON = true
			break
		}
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
		case "--config":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --config")
			}
			opts.ConfigPath = args[index]
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	return opts, nil
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := runOptions{}
	for _, arg := range args {
		if arg == "--json" {
			opts.JSON = true
			break
		}
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
		case "--dry-run":
			opts.DryRun = true
		case "--attach-run":
			opts.AttachRun = true
		case "--config":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --config")
			}
			opts.ConfigPath = args[index]
		case "--gate":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --gate")
			}
			opts.GateName = args[index]
		case "--profile":
			index++
			if index >= len(args) {
				return opts, fmt.Errorf("missing value for --profile")
			}
			opts.Profile = args[index]
		default:
			return opts, fmt.Errorf("unexpected argument: %s", args[index])
		}
	}
	if opts.GateName != "" && opts.Profile != "" {
		return opts, fmt.Errorf("--gate and --profile are mutually exclusive")
	}
	return opts, nil
}

func loadSet(opts options, environ []string) (Set, error) {
	if opts.ConfigPath != "" {
		return LoadFile(opts.ConfigPath, "explicit config")
	}

	wd, err := os.Getwd()
	if err != nil {
		return Set{}, err
	}
	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return Set{}, err
	}

	if resolved.Config.Loaded {
		return setFromAdapterConfig(resolved.Config), nil
	}
	return EmptySet(resolved.Config.Path), nil
}

func setFromAdapterConfig(config adapter.Config) Set {
	gates := make([]Gate, 0, len(config.Gates))
	for _, gate := range config.Gates {
		converted := Gate{
			Name:        gate.Name,
			Type:        gate.Type,
			RequiredFor: gate.RequiredFor,
			Description: gate.Description,
			Destructive: gate.Destructive,
		}
		if gate.Command != nil {
			converted.Command = &CommandMetadata{
				Argv:             gate.Command.Argv,
				WorkingDirectory: gate.Command.WorkingDirectory,
			}
		}
		if gate.Timeout != nil {
			converted.Timeout = &TimeoutMetadata{Seconds: gate.Timeout.Seconds}
		}
		gates = append(gates, converted)
	}
	set := Set{
		Source: Source{
			Path:   config.Path,
			Source: config.Source(),
			Loaded: config.Loaded,
		},
		Gates:    normalizeGates(gates),
		Profiles: profilesFromAdapterConfig(config),
	}
	set.Findings = validateGates(set.Gates)
	set.Summary = summarize(set.Gates, set.Findings)
	return set
}

func profilesFromAdapterConfig(config adapter.Config) map[string]Profile {
	if len(config.GateProfiles) == 0 {
		return nil
	}
	profiles := map[string]Profile{}
	for name, profile := range config.GateProfiles {
		profiles[name] = Profile{Required: profile.Required}
	}
	return normalizeProfiles(profiles)
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton gates list [--json] [--config PATH]
  anton gates check [--json] [--config PATH]
  anton gates run [--gate NAME|--profile NAME] [--dry-run] [--attach-run] [--json] [--config PATH]
`
}

func writeResponse(command string, set Set, asJSON bool, stdout io.Writer, ok bool) int {
	payload := response{
		OK:      ok,
		Command: command,
		Data:    &set,
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
	} else {
		renderHuman(stdout, command, set, ok)
	}
	if ok {
		return 0
	}
	return 1
}

func renderHuman(stdout io.Writer, command string, set Set, ok bool) {
	status := "blocked"
	if ok {
		status = "ok"
	}
	_, _ = fmt.Fprintf(stdout, "Anton %s\n", command)
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", status)
	_, _ = fmt.Fprintf(stdout, "Config: %s (%s)\n", set.Source.Source, set.Source.Path)
	_, _ = fmt.Fprintf(stdout, "Gates: declared=%d required=%d warnings=%d errors=%d\n", set.Summary.Declared, set.Summary.Required, set.Summary.Warnings, set.Summary.Errors)
	for _, gate := range set.Gates {
		_, _ = fmt.Fprintf(stdout, "- %s [%s]", gate.Name, gate.Type)
		if len(gate.RequiredFor) > 0 {
			_, _ = fmt.Fprintf(stdout, " required_for=%v", gate.RequiredFor)
		}
		_, _ = fmt.Fprintln(stdout)
	}
	for _, finding := range set.Findings {
		_, _ = fmt.Fprintf(stdout, "%s: %s", finding.Level, finding.Code)
		if finding.Gate != "" {
			_, _ = fmt.Fprintf(stdout, " gate=%s", finding.Gate)
		}
		if finding.Field != "" {
			_, _ = fmt.Fprintf(stdout, " field=%s", finding.Field)
		}
		_, _ = fmt.Fprintf(stdout, " - %s\n", finding.Message)
	}
}

func writeError(command string, code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	payload := response{
		OK:      false,
		Command: command,
		Error: &errorPayload{
			Code:    code,
			Message: message,
		},
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
