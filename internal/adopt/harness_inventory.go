package adopt

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Andrew0613/Anton/internal/contract"
	"github.com/Andrew0613/Anton/internal/doctor"
)

const (
	inventorySchemaVersion = 1

	classMoveToAnton     = "move_to_anton"
	classKeepProject     = "keep_project_local"
	classDeleteDeprecate = "delete_or_deprecate"
	classManualReview    = "manual_review"
)

type InventoryFinding struct {
	ID             string `json:"id"`
	Classification string `json:"classification"`
	Category       string `json:"category"`
	Path           string `json:"path"`
	Title          string `json:"title"`
	Evidence       string `json:"evidence"`
	Confidence     string `json:"confidence"`
	Recommendation string `json:"recommendation"`
}

type InventorySummary struct {
	FindingCount       int            `json:"finding_count"`
	InspectedPathCount int            `json:"inspected_path_count"`
	Classifications    map[string]int `json:"classifications"`
}

type HarnessInventoryReport struct {
	SchemaVersion    int                `json:"schema_version"`
	RepositoryRoot   string             `json:"repository_root"`
	WorkingDirectory string             `json:"working_directory"`
	ConfigSource     string             `json:"config_source"`
	Findings         []InventoryFinding `json:"findings"`
	Summary          InventorySummary   `json:"summary"`
}

type inventoryResponse struct {
	OK      bool                    `json:"ok"`
	Command string                  `json:"command"`
	Data    *HarnessInventoryReport `json:"data,omitempty"`
	Error   *errorPayload           `json:"error,omitempty"`
}

type inventoryOptions struct {
	Format string
}

type inventoryRule struct {
	ID              string
	Classification  string
	Category        string
	Title           string
	Confidence      string
	PathPatterns    []string
	ContentPatterns []string
	Recommendation  string
}

type inventoryCandidate struct {
	relPath string
	content string
}

var harnessInventoryRules = []inventoryRule{
	{
		ID:              "mandatory-planning-files",
		Classification:  classDeleteDeprecate,
		Category:        "planning_requirements",
		Title:           "Mandatory planning-file workflow",
		Confidence:      "high",
		PathPatterns:    []string{"AGENTS.md", "CLAUDE.md", "docs/**", "*.md"},
		ContentPatterns: []string{`(?i)planning-with-files`, `(?i)task_plan\.md`, `(?i)findings\.md`, `(?i)progress\.md`},
		Recommendation:  "replace mandatory planning-file language with Anton-native task or run state, keeping files only as optional projections",
	},
	{
		ID:              "local-task-state-script",
		Classification:  classMoveToAnton,
		Category:        "task_state",
		Title:           "Local task-state lifecycle script",
		Confidence:      "high",
		PathPatterns:    []string{"scripts/**", "tools/**", "bin/**", ".codex/**"},
		ContentPatterns: []string{`(?i)status\.yaml`, `(?i)task[-_ ]state`, `(?i)task_status`, `(?i)active_task`},
		Recommendation:  "move reusable lifecycle checks and mutations behind Anton task-state or run commands",
	},
	{
		ID:              "local-gate-wrapper",
		Classification:  classMoveToAnton,
		Category:        "gates",
		Title:           "Local validation or gate wrapper",
		Confidence:      "medium",
		PathPatterns:    []string{"scripts/**", "tools/**", "bin/**", ".github/workflows/**"},
		ContentPatterns: []string{`(?i)check_.*contracts`, `(?i)workflow[-_ ]contract`, `(?i)gate`, `(?i)preflight`, `(?i)validation receipt`},
		Recommendation:  "model generic validation wrappers as declarative Anton gates and receipts",
	},
	{
		ID:              "local-handoff-script",
		Classification:  classMoveToAnton,
		Category:        "handoff",
		Title:           "Local handoff or closure receipt surface",
		Confidence:      "medium",
		PathPatterns:    []string{"scripts/**", "tools/**", "docs/**", "*handoff*"},
		ContentPatterns: []string{`(?i)handoff`, `(?i)closure receipt`, `(?i)continuation`},
		Recommendation:  "prefer Anton handoff receipts for generic continuation context",
	},
	{
		ID:              "project-research-policy",
		Classification:  classKeepProject,
		Category:        "project_policy",
		Title:           "Project-specific research or infrastructure policy",
		Confidence:      "medium",
		PathPatterns:    []string{"docs/**", "scripts/**", "tools/**", "configs/**"},
		ContentPatterns: []string{`(?i)\bGPU\b`, `(?i)\bcluster\b`, `(?i)\bdataset\b`, `(?i)\bbenchmark\b`, `(?i)\bexperiment\b`, `(?i)\bmodel\b`, `(?i)\binference\b`},
		Recommendation:  "keep project-specific research, data, and infrastructure policy in the adopter repo",
	},
	{
		ID:              "legacy-harness-surface",
		Classification:  classDeleteDeprecate,
		Category:        "legacy_harness",
		Title:           "Legacy or deprecated harness surface",
		Confidence:      "medium",
		PathPatterns:    []string{"docs/**", "scripts/**", "tools/**", "*legacy*", "*deprecated*"},
		ContentPatterns: []string{`(?i)\blegacy\b`, `(?i)\bdeprecated\b`, `(?i)\bobsolete\b`, `(?i)remove after migration`},
		Recommendation:  "delete or deprecate this surface after Anton provides equivalent coverage",
	},
	{
		ID:              "hook-or-agent-config",
		Classification:  classManualReview,
		Category:        "hooks",
		Title:           "Hook or agent configuration",
		Confidence:      "medium",
		PathPatterns:    []string{".codex/**", ".claude/**", "AGENTS.md", "CLAUDE.md"},
		ContentPatterns: []string{`(?i)\bhook\b`, `(?i)pre_tool_use`, `(?i)post_tool_use`, `(?i)session_start`, `(?i)subagent`},
		Recommendation:  "review side effects manually before deciding whether Anton should own the behavior",
	},
	{
		ID:              "ambiguous-check-script",
		Classification:  classManualReview,
		Category:        "ambiguous_checker",
		Title:           "Ambiguous local checker",
		Confidence:      "low",
		PathPatterns:    []string{"scripts/check_*", "tools/check_*", "bin/check_*"},
		ContentPatterns: []string{},
		Recommendation:  "inspect whether this checker is generic harness behavior or project-specific policy",
	},
}

func runHarnessInventory(args []string, stdout io.Writer, stderr io.Writer, environ []string) int {
	opts, err := parseInventoryOptions(args)
	if err != nil {
		return writeInventoryError("usage", err.Error(), opts.Format == "json", stdout, stderr, 2)
	}

	contractData, err := doctor.CollectContract(environ)
	if err != nil {
		return writeInventoryError("contract", err.Error(), opts.Format == "json", stdout, stderr, 1)
	}

	report, err := AnalyzeHarnessInventory(contractData)
	if err != nil {
		return writeInventoryError("inventory", err.Error(), opts.Format == "json", stdout, stderr, 1)
	}

	if opts.Format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(inventoryResponse{
			OK:      true,
			Command: "adopt harness-inventory",
			Data:    &report,
		})
		return 0
	}

	renderHarnessInventoryMarkdown(stdout, report)
	return 0
}

func parseInventoryOptions(args []string) (inventoryOptions, error) {
	opts := inventoryOptions{Format: "markdown"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			if opts.Format != "markdown" {
				return opts, fmt.Errorf("--json and --format cannot be combined")
			}
			opts.Format = "json"
		case "--format":
			if opts.Format == "json" {
				return opts, fmt.Errorf("--json and --format cannot be combined")
			}
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--format requires a value")
			}
			i++
			if args[i] != "markdown" && args[i] != "json" {
				return opts, fmt.Errorf("unsupported format: %s", args[i])
			}
			opts.Format = args[i]
		default:
			return opts, fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	return opts, nil
}

func AnalyzeHarnessInventory(contractData contract.ContractV1) (HarnessInventoryReport, error) {
	base := contractData.Context.RepositoryRoot
	if strings.TrimSpace(base) == "" {
		base = contractData.Context.WorkingDirectory
	}
	base = filepath.Clean(base)

	candidates, inspected, err := collectInventoryCandidates(base)
	if err != nil {
		return HarnessInventoryReport{}, err
	}

	findings := []InventoryFinding{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		for _, rule := range harnessInventoryRules {
			if !ruleMatches(candidate, rule) {
				continue
			}
			key := rule.ID + "\x00" + candidate.relPath
			if seen[key] {
				continue
			}
			seen[key] = true
			findings = append(findings, InventoryFinding{
				ID:             rule.ID,
				Classification: rule.Classification,
				Category:       rule.Category,
				Path:           candidate.relPath,
				Title:          rule.Title,
				Evidence:       firstEvidence(candidate.content, rule.ContentPatterns),
				Confidence:     rule.Confidence,
				Recommendation: rule.Recommendation,
			})
		}
	}

	sort.Slice(findings, func(i int, j int) bool {
		if findings[i].Classification != findings[j].Classification {
			return classificationRank(findings[i].Classification) < classificationRank(findings[j].Classification)
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].ID < findings[j].ID
	})

	return HarnessInventoryReport{
		SchemaVersion:    inventorySchemaVersion,
		RepositoryRoot:   base,
		WorkingDirectory: contractData.Context.WorkingDirectory,
		ConfigSource:     contractData.Config.Source,
		Findings:         findings,
		Summary:          summarizeInventory(findings, inspected),
	}, nil
}

func collectInventoryCandidates(base string) ([]inventoryCandidate, int, error) {
	candidates := []inventoryCandidate{}
	inspected := 0
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}

		if entry.IsDir() {
			if shouldSkipInventoryDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isInventoryCandidatePath(rel) {
			return nil
		}

		inspected++
		content := ""
		info, statErr := entry.Info()
		if statErr == nil && info.Size() <= 256*1024 {
			if bytes, readErr := os.ReadFile(path); readErr == nil {
				content = string(bytes)
			}
		}
		candidates = append(candidates, inventoryCandidate{relPath: rel, content: content})
		return nil
	})
	return candidates, inspected, err
}

func shouldSkipInventoryDir(rel string) bool {
	switch rel {
	case ".git", ".worktrees", "node_modules", "vendor", "tmp", "dist", "build", ".cache":
		return true
	}
	return strings.HasPrefix(rel, ".anton/history") || strings.Contains(rel, "/.git/")
}

func isInventoryCandidatePath(rel string) bool {
	for _, rule := range harnessInventoryRules {
		for _, pattern := range rule.PathPatterns {
			if matchInventoryPattern(pattern, rel) {
				return true
			}
		}
	}
	return false
}

func ruleMatches(candidate inventoryCandidate, rule inventoryRule) bool {
	pathMatched := false
	for _, pattern := range rule.PathPatterns {
		if matchInventoryPattern(pattern, candidate.relPath) {
			pathMatched = true
			break
		}
	}
	if !pathMatched {
		return false
	}

	if len(rule.ContentPatterns) == 0 {
		return true
	}
	for _, pattern := range rule.ContentPatterns {
		if regexp.MustCompile(pattern).FindStringIndex(candidate.content) != nil {
			return true
		}
	}
	return false
}

func matchInventoryPattern(pattern string, rel string) bool {
	pattern = filepath.ToSlash(pattern)
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	if strings.Contains(pattern, "*") {
		matched, err := filepath.Match(pattern, rel)
		return err == nil && matched
	}
	return rel == pattern
}

func firstEvidence(content string, patterns []string) string {
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		loc := re.FindStringIndex(content)
		if loc == nil {
			continue
		}
		return compactEvidence(content, loc[0], loc[1])
	}
	return "path matched inventory rule"
}

func compactEvidence(content string, start int, end int) string {
	lineStart := strings.LastIndex(content[:start], "\n") + 1
	lineEnd := strings.Index(content[end:], "\n")
	if lineEnd == -1 {
		lineEnd = len(content)
	} else {
		lineEnd += end
	}
	line := strings.TrimSpace(content[lineStart:lineEnd])
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > 160 {
		line = strings.TrimSpace(line[:157]) + "..."
	}
	return line
}

func summarizeInventory(findings []InventoryFinding, inspected int) InventorySummary {
	counts := map[string]int{
		classMoveToAnton:     0,
		classKeepProject:     0,
		classDeleteDeprecate: 0,
		classManualReview:    0,
	}
	for _, finding := range findings {
		counts[finding.Classification]++
	}
	return InventorySummary{
		FindingCount:       len(findings),
		InspectedPathCount: inspected,
		Classifications:    counts,
	}
}

func classificationRank(classification string) int {
	switch classification {
	case classMoveToAnton:
		return 0
	case classKeepProject:
		return 1
	case classDeleteDeprecate:
		return 2
	case classManualReview:
		return 3
	default:
		return 4
	}
}

func renderHarnessInventoryMarkdown(stdout io.Writer, report HarnessInventoryReport) {
	_, _ = fmt.Fprintf(stdout, "# Anton Harness Inventory\n\n")
	_, _ = fmt.Fprintf(stdout, "- Repository: `%s`\n", report.RepositoryRoot)
	_, _ = fmt.Fprintf(stdout, "- Config: `%s`\n", report.ConfigSource)
	_, _ = fmt.Fprintf(stdout, "- Findings: `%d`\n", report.Summary.FindingCount)
	_, _ = fmt.Fprintf(stdout, "- Inspected paths: `%d`\n\n", report.Summary.InspectedPathCount)

	if len(report.Findings) == 0 {
		_, _ = fmt.Fprintf(stdout, "No harness inventory findings.\n")
		return
	}

	for _, classification := range []string{classMoveToAnton, classKeepProject, classDeleteDeprecate, classManualReview} {
		group := findingsByClassification(report.Findings, classification)
		if len(group) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "## %s\n\n", classification)
		for _, finding := range group {
			_, _ = fmt.Fprintf(stdout, "- `%s` `%s`: %s\n", finding.Path, finding.ID, finding.Title)
			_, _ = fmt.Fprintf(stdout, "  - Evidence: %s\n", finding.Evidence)
			_, _ = fmt.Fprintf(stdout, "  - Recommendation: %s\n", finding.Recommendation)
		}
		_, _ = fmt.Fprintf(stdout, "\n")
	}
}

func findingsByClassification(findings []InventoryFinding, classification string) []InventoryFinding {
	group := []InventoryFinding{}
	for _, finding := range findings {
		if finding.Classification == classification {
			group = append(group, finding)
		}
	}
	return group
}

func writeInventoryError(code string, message string, asJSON bool, stdout io.Writer, stderr io.Writer, exitCode int) int {
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(inventoryResponse{
			OK:      false,
			Command: "adopt harness-inventory",
			Error: &errorPayload{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	_, _ = fmt.Fprintf(stderr, "%s\n", message)
	return exitCode
}
