package entrypoint

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const (
	defaultLineBudget = 120
	statusOK          = "ok"
	statusDegraded    = "degraded"
	statusBlocked     = "blocked"
)

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

type options struct {
	JSON bool
}

type configContract struct {
	Path           string `json:"path"`
	Source         string `json:"source"`
	EntrypointPath string `json:"entrypoint_path"`
	LineBudget     int    `json:"line_budget"`
}

type finding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type reference struct {
	Target  string `json:"target"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
	Link    string `json:"link,omitempty"`
	Line    int    `json:"line,omitempty"`
	AbsPath string `json:"abs_path,omitempty"`
}

type summary struct {
	Status        string `json:"status"`
	OKCount       int    `json:"ok_count"`
	DegradedCount int    `json:"degraded_count"`
	BlockedCount  int    `json:"blocked_count"`
}

type commandData struct {
	Adapter          string         `json:"adapter"`
	WorkingDirectory string         `json:"working_directory"`
	Config           configContract `json:"config"`
	Exists           bool           `json:"exists"`
	LineCount        int            `json:"line_count"`
	References       []reference    `json:"references,omitempty"`
	Findings         []finding      `json:"findings,omitempty"`
	Summary          summary        `json:"summary"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    *commandData  `json:"data,omitempty"`
	Error   *errorPayload `json:"error,omitempty"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	if len(args) == 0 {
		return writeUsage(stderr)
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr, environ)
	case "sync":
		opts, err := parseOptions(args[1:])
		if err != nil {
			return writeError("entrypoint sync", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
		}
		return writeError("entrypoint sync", "not-approved", "entrypoint sync is not approved until the separate sync safety plan lands", opts.JSON, stdout, stderr, 2)
	case "help", "-h", "--help":
		_, _ = io.WriteString(stdout, usageText())
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown entrypoint command: %s\n\n%s", args[0], usageText())
		return 2
	}
}

func runCheck(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		return writeError("entrypoint check", "usage", err.Error(), opts.JSON, stdout, stderr, 2)
	}

	wd, err := os.Getwd()
	if err != nil {
		return writeError("entrypoint check", "entrypoint-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	resolved, err := adapter.Resolve(wd, environ)
	if err != nil {
		return writeError("entrypoint check", "entrypoint-check-failed", err.Error(), opts.JSON, stdout, stderr, 1)
	}

	entrypointPath := resolved.Definition.EntrypointPath(resolved.Context)
	findings, refs, exists, lineCount := checkEntrypoint(entrypointPath, resolved.Context.RepositoryRoot, defaultLineBudget)
	data := commandData{
		Adapter:          resolved.Definition.Name(),
		WorkingDirectory: wd,
		Config: configContract{
			Path:           resolved.Config.Path,
			Source:         resolved.Config.Source(),
			EntrypointPath: entrypointPath,
			LineBudget:     defaultLineBudget,
		},
		Exists:     exists,
		LineCount:  lineCount,
		References: refs,
		Findings:   findings,
		Summary:    summarize(findings),
	}

	exitCode := 0
	if data.Summary.Status != statusOK {
		exitCode = 1
	}
	return writeResponse("entrypoint check", data, opts.JSON, stdout, exitCode)
}

func checkEntrypoint(path string, repoRoot string, budget int) ([]finding, []reference, bool, int) {
	findings := []finding{}
	refs := []reference{}

	info, err := os.Stat(path)
	if err != nil {
		findings = append(findings, finding{
			Level:   statusBlocked,
			Code:    "primary-entrypoint-missing",
			Path:    path,
			Message: "configured entrypoint is missing",
			Hint:    "create the configured repo-local entrypoint before using it as an agent contract",
		})
		return findings, refs, false, 0
	}
	if info.IsDir() {
		findings = append(findings, finding{
			Level:   statusBlocked,
			Code:    "primary-entrypoint-invalid",
			Path:    path,
			Message: "configured entrypoint is a directory, expected a file",
		})
		return findings, refs, false, 0
	}

	content, err := os.ReadFile(path)
	if err != nil {
		findings = append(findings, finding{
			Level:   statusBlocked,
			Code:    "primary-entrypoint-unreadable",
			Path:    path,
			Message: fmt.Sprintf("read configured entrypoint: %v", err),
		})
		return findings, refs, true, 0
	}

	lineCount := countLines(content)
	if lineCount > budget {
		findings = append(findings, finding{
			Level:   statusDegraded,
			Code:    "entrypoint-over-budget",
			Path:    path,
			Message: fmt.Sprintf("configured entrypoint has %d lines, above the %d line first-slice budget", lineCount, budget),
			Hint:    "keep the entrypoint short and route detailed design to docs/",
		})
	}

	if repoRoot != "" && filepath.Base(path) != "README.md" {
		readmePath := filepath.Join(repoRoot, "README.md")
		ref := reference{
			Target:  "README.md",
			AbsPath: readmePath,
		}
		switch {
		case !strings.Contains(string(content), "README.md"):
			ref.Status = statusBlocked
			ref.Detail = "configured entrypoint does not reference README.md"
			findings = append(findings, finding{
				Level:   statusBlocked,
				Code:    "missing-readme-reference",
				Path:    path,
				Message: "configured entrypoint must route readers to README.md",
			})
		case !fileExists(readmePath):
			ref.Status = statusBlocked
			ref.Detail = "README.md is referenced but missing"
			findings = append(findings, finding{
				Level:   statusBlocked,
				Code:    "missing-readme-file",
				Path:    readmePath,
				Message: "README.md is referenced by the entrypoint but does not exist",
			})
		default:
			ref.Status = statusOK
			ref.Detail = "README.md reference resolved"
		}
		refs = append(refs, ref)
	}

	linkRefs, linkFindings := checkMarkdownLinks(path, repoRoot, content)
	refs = append(refs, linkRefs...)
	findings = append(findings, linkFindings...)
	return findings, refs, true, lineCount
}

func checkMarkdownLinks(filePath string, repoRoot string, content []byte) ([]reference, []finding) {
	refs := []reference{}
	findings := []finding{}
	if strings.TrimSpace(repoRoot) == "" {
		return refs, findings
	}

	lines := strings.Split(string(content), "\n")
	for lineIndex, line := range lines {
		matches := markdownLinkPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) != 2 {
				continue
			}
			link := strings.TrimSpace(match[1])
			if skipLink(link) {
				continue
			}
			target := filepath.Clean(filepath.Join(filepath.Dir(filePath), link))
			ref := reference{
				Target:  filepath.ToSlash(target),
				Link:    link,
				Line:    lineIndex + 1,
				AbsPath: target,
			}
			if !pathWithinRoot(repoRoot, target) {
				ref.Status = statusDegraded
				ref.Detail = "relative markdown link escapes repository"
				findings = append(findings, finding{
					Level:   statusDegraded,
					Code:    "link-escapes-repo",
					Path:    filePath,
					Message: fmt.Sprintf("relative markdown link escapes repository: %s", link),
				})
			} else if !fileExists(target) {
				ref.Status = statusDegraded
				ref.Detail = "relative markdown link target is missing"
				findings = append(findings, finding{
					Level:   statusDegraded,
					Code:    "stale-link",
					Path:    filePath,
					Message: fmt.Sprintf("relative markdown link target is missing: %s", link),
				})
			} else {
				ref.Status = statusOK
				ref.Detail = "relative markdown link resolved"
			}
			refs = append(refs, ref)
		}
	}
	return refs, findings
}

func skipLink(link string) bool {
	return link == "" || strings.HasPrefix(link, "#") || strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:")
}

func countLines(content []byte) int {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathWithinRoot(root string, path string) bool {
	rootClean := filepath.Clean(root)
	pathClean := filepath.Clean(path)
	relative, err := filepath.Rel(rootClean, pathClean)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func summarize(findings []finding) summary {
	result := summary{Status: statusOK}
	if len(findings) == 0 {
		result.OKCount = 1
		return result
	}
	for _, item := range findings {
		switch item.Level {
		case statusBlocked:
			result.BlockedCount++
			result.Status = statusBlocked
		case statusDegraded:
			result.DegradedCount++
			if result.Status != statusBlocked {
				result.Status = statusDegraded
			}
		default:
			result.OKCount++
		}
	}
	return result
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.JSON = true
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func writeUsage(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usageText())
	return 2
}

func usageText() string {
	return `Usage:
  anton entrypoint check [--json]
  anton entrypoint sync [--json]
`
}

func writeResponse(command string, data commandData, asJSON bool, stdout io.Writer, exitCode int) int {
	payload := response{
		OK:      data.Summary.Status == statusOK,
		Command: command,
		Data:    &data,
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(payload)
		return exitCode
	}

	_, _ = fmt.Fprintf(stdout, "Anton %s\n", command)
	_, _ = fmt.Fprintf(stdout, "Status: %s\n", data.Summary.Status)
	_, _ = fmt.Fprintf(stdout, "Entrypoint: %s\n", data.Config.EntrypointPath)
	_, _ = fmt.Fprintf(stdout, "Lines: %d/%d\n", data.LineCount, data.Config.LineBudget)
	for _, item := range data.Findings {
		_, _ = fmt.Fprintf(stdout, "  %-8s %s: %s\n", strings.ToUpper(item.Level), item.Code, item.Message)
	}
	return exitCode
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
