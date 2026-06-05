package policy

import (
	"testing"
)

func baseRule(kind string) Rule {
	return Rule{
		RuleID:   "test-rule",
		Owner:    "harness",
		Severity: "error",
		Check: CheckSpec{
			Kind: kind,
		},
	}
}

func hasIssueCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestValidateCheckSpec_FileContainsAll_MissingTokens(t *testing.T) {
	rule := baseRule("file_contains_all")
	rule.Check.Path = "some/file.md"
	// Tokens is empty — should produce policy-rule-check-tokens-missing
	issues := validateCheckSpec(rule, "test-rule", "test.yaml")
	if !hasIssueCode(issues, "policy-rule-check-tokens-missing") {
		t.Errorf("expected policy-rule-check-tokens-missing, got: %+v", issues)
	}
}

func TestValidateCheckSpec_FileContainsAll_Valid(t *testing.T) {
	rule := baseRule("file_contains_all")
	rule.Check.Path = "some/file.md"
	rule.Check.Tokens = []string{"foo", "bar"}
	issues := validateCheckSpec(rule, "test-rule", "test.yaml")
	if hasIssueCode(issues, "policy-rule-check-tokens-missing") {
		t.Errorf("unexpected policy-rule-check-tokens-missing issue")
	}
	if hasIssueCode(issues, "policy-rule-check-path-missing") {
		t.Errorf("unexpected policy-rule-check-path-missing issue")
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %+v", issues)
	}
}

func TestValidateCheckSpec_MarkdownHasSections_MissingSections(t *testing.T) {
	rule := baseRule("markdown_has_sections")
	rule.Check.Path = "some/doc.md"
	// Sections is empty — should produce policy-rule-check-sections-missing
	issues := validateCheckSpec(rule, "test-rule", "test.yaml")
	if !hasIssueCode(issues, "policy-rule-check-sections-missing") {
		t.Errorf("expected policy-rule-check-sections-missing, got: %+v", issues)
	}
}

func TestValidateCheckSpec_MarkdownHasSections_PathPatternValid(t *testing.T) {
	rule := baseRule("markdown_has_sections")
	rule.Check.PathPattern = "docs/**/*.md"
	rule.Check.Sections = []string{"## Overview", "## Usage"}
	issues := validateCheckSpec(rule, "test-rule", "test.yaml")
	if hasIssueCode(issues, "policy-rule-check-path-missing") {
		t.Errorf("unexpected policy-rule-check-path-missing issue")
	}
	if hasIssueCode(issues, "policy-rule-check-sections-missing") {
		t.Errorf("unexpected policy-rule-check-sections-missing issue")
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %+v", issues)
	}
}

func TestValidateCheckSpec_PathExists_NoNewFields(t *testing.T) {
	rule := baseRule("path_exists")
	rule.Check.Path = "AGENTS.md"
	// No new fields set; existing path_exists behavior unchanged
	issues := validateCheckSpec(rule, "test-rule", "test.yaml")
	if len(issues) != 0 {
		t.Errorf("expected no issues for path_exists, got: %+v", issues)
	}
}

func TestValidateRules_DuplicateRuleID(t *testing.T) {
	rules := []Rule{
		{
			RuleID:   "duplicate-id",
			Owner:    "harness",
			Severity: "error",
			Check:    CheckSpec{Kind: "path_exists", Path: "AGENTS.md"},
		},
		{
			RuleID:   "duplicate-id",
			Owner:    "harness",
			Severity: "error",
			Check:    CheckSpec{Kind: "path_exists", Path: "AGENTS.md"},
		},
	}
	issues := validateRules(rules, "test.yaml")
	if !hasIssueCode(issues, "policy-rule-id-duplicate") {
		t.Errorf("expected policy-rule-id-duplicate, got: %+v", issues)
	}
}
