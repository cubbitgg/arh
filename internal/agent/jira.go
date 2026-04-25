package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/jira"
	"github.com/cubbitgg/arh/internal/llm"
)

// JiraAgent validates PR changes against the linked Jira issue's acceptance criteria.
type JiraAgent struct {
	BaseAgent
	jiraClient   jira.JiraClient
	issuePattern *regexp.Regexp
}

// NewJiraAgent creates a Jira agent. Returns an error if the issue pattern regex is invalid.
func NewJiraAgent(cfg *config.Config, llmClient llm.LLMClient, jiraClient jira.JiraClient) (*JiraAgent, error) {
	pattern := cfg.Jira.IssuePattern
	if pattern == "" {
		pattern = `[A-Z]{2,10}-\d+`
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compiling Jira issue pattern %q: %w", pattern, err)
	}
	return &JiraAgent{
		BaseAgent:    newBaseAgent("jira", jiraBuiltinPrompt, findingOutputFormat, cfg, llmClient),
		jiraClient:   jiraClient,
		issuePattern: re,
	}, nil
}

// Run extracts a Jira issue key from the PR description, fetches the issue, and
// uses the LLM to compare the PR changes against the issue's acceptance criteria.
func (a *JiraAgent) Run(ctx context.Context, pr *github.PRData) ([]Finding, error) {
	if strings.TrimSpace(pr.Body) == "" {
		return []Finding{{
			Agent:      "jira",
			Severity:   SeverityWarning,
			RuleID:     "jira-no-description",
			Message:    "PR description is empty; cannot extract a Jira issue key",
			Suggestion: "Add a Jira issue key (e.g., PROJ-123) to the PR description",
		}}, nil
	}

	issueKey := a.issuePattern.FindString(pr.Body)
	if issueKey == "" {
		return []Finding{{
			Agent:      "jira",
			Severity:   SeverityWarning,
			RuleID:     "jira-no-key",
			Message:    fmt.Sprintf("No Jira issue key matching pattern %q found in PR description", a.issuePattern.String()),
			Suggestion: "Add a Jira issue key (e.g., PROJ-123) to the PR description",
		}}, nil
	}

	issue, err := a.jiraClient.FetchIssue(ctx, issueKey)
	if err != nil {
		return []Finding{{
			Agent:    "jira",
			Severity: SeverityWarning,
			RuleID:   "jira-fetch-error",
			Message:  fmt.Sprintf("Failed to fetch Jira issue %s: %v", issueKey, err),
		}}, nil
	}

	userPrompt := a.buildUserPrompt(pr, issue, issueKey)
	resp, err := a.completeLLM(ctx, userPrompt)
	if err != nil {
		return []Finding{{
			Agent:    "jira",
			Severity: SeverityWarning,
			RuleID:   "jira-llm-error",
			Message:  fmt.Sprintf("LLM analysis failed for Jira issue %s: %v", issueKey, err),
		}}, nil
	}

	return ParseFindings("jira", resp), nil
}

func (a *JiraAgent) buildUserPrompt(pr *github.PRData, issue *jira.Issue, issueKey string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Jira Issue: %s\n", issueKey)
	fmt.Fprintf(&sb, "Summary: %s\n", issue.Summary)
	fmt.Fprintf(&sb, "Status: %s | Type: %s\n", issue.Status, issue.IssueType)
	if issue.Description != "" {
		fmt.Fprintf(&sb, "Description:\n%s\n", truncate(issue.Description, 2000))
	}
	if issue.AcceptanceCriteria != "" {
		fmt.Fprintf(&sb, "Acceptance Criteria:\n%s\n", truncate(issue.AcceptanceCriteria, 1500))
	} else {
		fmt.Fprintln(&sb, "Acceptance Criteria: (none specified in Jira)")
	}

	fmt.Fprintf(&sb, "\n---\n\nPR #%d: %s\n", pr.Number, pr.Title)
	if pr.Body != "" {
		fmt.Fprintf(&sb, "PR Description:\n%s\n", truncate(pr.Body, 1000))
	}

	fmt.Fprintf(&sb, "\nChanged files (%d total):\n", len(pr.ChangedFiles))
	for _, f := range pr.ChangedFiles {
		fmt.Fprintf(&sb, "  %s  +%d -%d  [%s]\n", f.Path, f.Additions, f.Deletions, f.Status)
	}

	// Include a condensed diff summary for context.
	fmt.Fprintln(&sb, "\nCondensed diff:")
	for _, f := range pr.ChangedFiles {
		diff := pr.PerFileDiff[f.Path]
		if diff == "" {
			continue
		}
		condensed := condenseDiff(diff, 10)
		fmt.Fprintf(&sb, "\n--- %s ---\n%s\n", f.Path, condensed)
		if sb.Len() > 8000 {
			fmt.Fprintln(&sb, "(truncated — remaining files omitted for brevity)")
			break
		}
	}

	return sb.String()
}
