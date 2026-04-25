package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
)

// RulesAgent checks PR metadata: branch name, commit messages, labels, description.
type RulesAgent struct {
	BaseAgent
}

// NewRulesAgent creates a Rules agent.
func NewRulesAgent(cfg *config.Config, llmClient llm.LLMClient) *RulesAgent {
	return &RulesAgent{
		BaseAgent: newBaseAgent("rules", rulesBuiltinPrompt, findingOutputFormat, cfg, llmClient),
	}
}

// Run performs deterministic checks and then enriches violations via LLM.
func (a *RulesAgent) Run(ctx context.Context, pr *github.PRData) ([]Finding, error) {
	violations := a.deterministic(pr)
	if len(violations) == 0 && !a.cfg.Rules.PRDescriptionRequired {
		// Still run LLM for title accuracy
	}

	findings, err := a.llmEnrichment(ctx, pr, violations)
	if err != nil {
		return violations, nil // return deterministic findings even if LLM fails
	}
	return findings, nil
}

// deterministic checks that can be evaluated without an LLM.
func (a *RulesAgent) deterministic(pr *github.PRData) []Finding {
	var findings []Finding
	cfg := a.cfg.Rules

	// Branch name pattern
	if cfg.BranchPattern != "" {
		re, err := regexp.Compile(cfg.BranchPattern)
		if err == nil && !re.MatchString(pr.BranchName) {
			findings = append(findings, Finding{
				Agent:    "rules",
				Severity: SeverityError,
				RuleID:   "branch-pattern",
				Message:  fmt.Sprintf("Branch name %q does not match required pattern %s", pr.BranchName, cfg.BranchPattern),
			})
		}
	}

	// Conventional commits
	if cfg.RequireConventionalCommits {
		ccRe := regexp.MustCompile(`^(feat|fix|chore|refactor|docs|test|ci|perf|style|build|revert)(\([^)]+\))?!?: .+`)
		for _, c := range pr.Commits {
			if !ccRe.MatchString(c.Subject) {
				findings = append(findings, Finding{
					Agent:    "rules",
					Severity: SeverityError,
					RuleID:   "conventional-commits",
					Message:  fmt.Sprintf("Commit %q does not follow Conventional Commits format", c.Subject),
				})
			}
		}
	}

	// Required labels
	labelSet := make(map[string]bool)
	for _, l := range pr.Labels {
		labelSet[l] = true
	}
	for _, required := range cfg.RequiredLabels {
		if !labelSet[required] {
			findings = append(findings, Finding{
				Agent:      "rules",
				Severity:   SeverityWarning,
				RuleID:     "required-label",
				Message:    fmt.Sprintf("PR is missing required label %q", required),
				Suggestion: fmt.Sprintf("Add label %q to the PR", required),
			})
		}
	}

	// PR title length
	if cfg.PRTitleMaxLength > 0 && len(pr.Title) > cfg.PRTitleMaxLength {
		findings = append(findings, Finding{
			Agent:    "rules",
			Severity: SeverityWarning,
			RuleID:   "pr-title-length",
			Message:  fmt.Sprintf("PR title is %d characters, exceeds limit of %d", len(pr.Title), cfg.PRTitleMaxLength),
		})
	}

	// PR description required
	if cfg.PRDescriptionRequired && strings.TrimSpace(pr.Body) == "" {
		findings = append(findings, Finding{
			Agent:      "rules",
			Severity:   SeverityWarning,
			RuleID:     "pr-description-required",
			Message:    "PR description is empty",
			Suggestion: "Add a description explaining what this PR does and why",
		})
	}

	return findings
}

// llmEnrichment asks the LLM to suggest fixes for violations and check title accuracy.
func (a *RulesAgent) llmEnrichment(ctx context.Context, pr *github.PRData, violations []Finding) ([]Finding, error) {
	// Build user prompt combining PR metadata and existing violations
	var sb strings.Builder
	fmt.Fprintf(&sb, "PR #%d: %s\n", pr.Number, pr.Title)
	fmt.Fprintf(&sb, "Branch: %s\n", pr.BranchName)
	fmt.Fprintf(&sb, "Labels: %s\n", strings.Join(pr.Labels, ", "))
	fmt.Fprintf(&sb, "Description:\n%s\n\n", pr.Body)
	fmt.Fprintf(&sb, "Commits:\n")
	for _, c := range pr.Commits {
		fmt.Fprintf(&sb, "  - %s\n", c.Subject)
	}

	if len(violations) > 0 {
		fmt.Fprintf(&sb, "\nDeterministic violations already found (suggest concrete fixes for each):\n")
		for _, v := range violations {
			fmt.Fprintf(&sb, "  [%s] %s\n", v.RuleID, v.Message)
		}
	}

	// Add a short diff summary for title accuracy check
	lines := strings.Split(pr.Diff, "\n")
	summary := make([]string, 0, 20)
	for _, l := range lines {
		if strings.HasPrefix(l, "diff --git") || strings.HasPrefix(l, "+++") {
			summary = append(summary, l)
			if len(summary) >= 20 {
				break
			}
		}
	}
	fmt.Fprintf(&sb, "\nChanged files summary:\n%s\n", strings.Join(summary, "\n"))
	fmt.Fprintf(&sb, "\nAlso check: does the PR title accurately describe what the diff changes?\n")

	resp, err := a.completeLLM(ctx, sb.String())
	if err != nil {
		return violations, err
	}

	llmFindings := ParseFindings("rules", resp)
	// Merge: LLM findings override deterministic ones when they share the same RuleID
	seen := map[string]bool{}
	for _, f := range llmFindings {
		seen[f.RuleID] = true
	}
	for _, v := range violations {
		if !seen[v.RuleID] {
			llmFindings = append(llmFindings, v)
		}
	}
	return llmFindings, nil
}
