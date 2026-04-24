package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
)

// FocusAgent ranks changed files by review priority.
type FocusAgent struct {
	BaseAgent
}

// NewFocusAgent creates a Focus agent.
func NewFocusAgent(cfg *config.Config, llmClient llm.LLMClient) *FocusAgent {
	return &FocusAgent{
		BaseAgent: newBaseAgent("focus", focusBuiltinPrompt, fileFocusOutputFormat, cfg, llmClient),
	}
}

// Run builds a diff summary and asks the LLM to rank files by review priority.
// It returns findings (one per file) plus stores FileFocus items internally.
func (a *FocusAgent) Run(ctx context.Context, pr *github.PRData) ([]Finding, error) {
	if len(pr.ChangedFiles) == 0 {
		return nil, nil
	}

	userPrompt := a.buildPrompt(pr)
	resp, err := a.completeLLM(ctx, userPrompt)
	if err != nil {
		return nil, err
	}

	items := ParseFileFocus(resp)
	if len(items) == 0 {
		return nil, nil
	}

	findings := make([]Finding, 0, len(items))
	for _, item := range items {
		sev := SeverityInfo
		if item.Priority == "high" {
			sev = SeverityWarning
		}
		findings = append(findings, Finding{
			Agent:    "focus",
			Severity: sev,
			File:     item.File,
			RuleID:   "focus-" + item.Priority,
			Message:  fmt.Sprintf("[%s priority] %s", item.Priority, item.Reason),
		})
	}
	return findings, nil
}

func (a *FocusAgent) buildPrompt(pr *github.PRData) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "PR: %s\n", pr.Title)
	fmt.Fprintf(&sb, "Changed files (%d total):\n", len(pr.ChangedFiles))
	for _, f := range pr.ChangedFiles {
		fmt.Fprintf(&sb, "  %s  +%d -%d  [%s]\n", f.Path, f.Additions, f.Deletions, f.Status)
	}
	fmt.Fprintf(&sb, "\nCondensed diff (first few hunks per file):\n")
	for _, f := range pr.ChangedFiles {
		diff := pr.PerFileDiff[f.Path]
		if diff == "" {
			continue
		}
		condensed := condenseDiff(diff, 5)
		fmt.Fprintf(&sb, "\n--- %s ---\n%s\n", f.Path, condensed)
	}
	return sb.String()
}

// condenseDiff returns up to maxHunkLines lines from each hunk block.
func condenseDiff(diff string, maxHunkLines int) string {
	var out strings.Builder
	lines := strings.Split(diff, "\n")
	hunkCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			hunkCount = 0
			out.WriteString(line + "\n")
			continue
		}
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			out.WriteString(line + "\n")
			continue
		}
		if hunkCount < maxHunkLines {
			out.WriteString(line + "\n")
			hunkCount++
		}
	}
	return out.String()
}
