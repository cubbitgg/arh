package report

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// MarkdownRenderer writes the report as a markdown file.
type MarkdownRenderer struct {
	pathTemplate string
}

// NewMarkdownRenderer creates a renderer that writes to the resolved path.
func NewMarkdownRenderer(pathTemplate string) *MarkdownRenderer {
	return &MarkdownRenderer{pathTemplate: pathTemplate}
}

// Render writes the report to a markdown file.
func (r *MarkdownRenderer) Render(rep *orchestrator.Report) error {
	path := resolveOutputPath(r.pathTemplate, rep.PR.Number)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating markdown file %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	pr := rep.PR

	fmt.Fprintf(w, "# PR Review: #%d — %s\n\n", pr.Number, pr.Title)
	fmt.Fprintf(w, "**Branch:** `%s`  \n", pr.BranchName)
	fmt.Fprintf(w, "**Date:** %s\n\n", time.Now().Format("2006-01-02"))

	errors, warnings, infos := countSeverities(rep.Findings)
	fmt.Fprintf(w, "## Summary\n\n")
	fmt.Fprintf(w, "| Severity | Count |\n")
	fmt.Fprintf(w, "|----------|-------|\n")
	fmt.Fprintf(w, "| Error    | %d     |\n", errors)
	fmt.Fprintf(w, "| Warning  | %d     |\n", warnings)
	fmt.Fprintf(w, "| Info     | %d     |\n\n", infos)

	focusItems := filterByAgent(rep.Findings, "focus")
	if len(focusItems) > 0 {
		fmt.Fprintf(w, "## Files to Review\n\n")
		sortByPriority(focusItems)
		for _, item := range focusItems {
			priority := "low"
			if strings.HasSuffix(item.RuleID, "high") {
				priority = "high"
			} else if strings.HasSuffix(item.RuleID, "medium") {
				priority = "medium"
			}
			fmt.Fprintf(w, "- **%s** (%s priority) — %s\n", item.File, priority, item.Message)
		}
		fmt.Fprintln(w)
	}

	agentOrder := []string{"rules", "lint", "logic", "jira"}
	for _, agentName := range agentOrder {
		agentFindings := filterByAgent(rep.Findings, agentName)
		if len(agentFindings) == 0 {
			continue
		}
		fmt.Fprintf(w, "## %s Agent\n\n", strings.ToUpper(agentName[:1])+agentName[1:])
		sortBySeverity(agentFindings)
		for _, finding := range agentFindings {
			writeMarkdownFinding(w, finding)
		}
	}

	return w.Flush()
}

func writeMarkdownFinding(w *bufio.Writer, f agent.Finding) {
	loc := formatLocation(f)
	header := fmt.Sprintf("[%s]", f.Severity)
	if f.RuleID != "" {
		header += fmt.Sprintf(" `%s`", f.RuleID)
	}
	if loc != "" {
		header += fmt.Sprintf(" — `%s`", loc)
	}
	fmt.Fprintf(w, "### %s\n\n", header)
	fmt.Fprintf(w, "%s\n\n", f.Message)
	if f.Suggestion != "" {
		if strings.Contains(f.Suggestion, "\n") {
			fmt.Fprintf(w, "> **Suggestion:**\n> ```\n")
			for _, line := range strings.Split(f.Suggestion, "\n") {
				fmt.Fprintf(w, "> %s\n", line)
			}
			fmt.Fprintln(w, "> ```")
		} else {
			fmt.Fprintf(w, "> **Suggestion:** %s\n", f.Suggestion)
		}
		fmt.Fprintln(w)
	}
}
