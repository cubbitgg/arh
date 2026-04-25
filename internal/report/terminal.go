package report

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// TerminalRenderer renders a report to the terminal using lipgloss colors.
type TerminalRenderer struct {
	w            io.Writer
	colorEnabled bool
}

// NewTerminalRenderer creates a renderer that writes to stdout and auto-detects TTY.
func NewTerminalRenderer() *TerminalRenderer {
	return &TerminalRenderer{
		w:            os.Stdout,
		colorEnabled: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

// NewTerminalRendererTo creates a renderer that writes to w (useful for testing).
func NewTerminalRendererTo(w io.Writer) *TerminalRenderer {
	return &TerminalRenderer{w: w}
}

// Render writes the full report to the terminal.
func (r *TerminalRenderer) Render(rep *orchestrator.Report) error {
	if r.colorEnabled {
		r.renderColor(rep)
	} else {
		r.renderPlain(rep)
	}
	return nil
}

func (r *TerminalRenderer) renderColor(rep *orchestrator.Report) {
	boldStyle := lipgloss.NewStyle().Bold(true)
	headerStyle := lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("39"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	suggStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	w := r.w
	pr := rep.PR

	fmt.Fprintln(w, headerStyle.Render(fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)))
	fmt.Fprintln(w, dimStyle.Render(fmt.Sprintf("  %s → %s", pr.BranchName, pr.BaseSHA[:8])))

	errors, warnings, infos := countSeverities(rep.Findings)
	fmt.Fprintf(w, "  %s  %s  %s\n\n",
		errorStyle.Render(fmt.Sprintf("✖ %d errors", errors)),
		warningStyle.Render(fmt.Sprintf("⚠ %d warnings", warnings)),
		infoStyle.Render(fmt.Sprintf("ℹ %d info", infos)),
	)

	focusItems := filterByAgent(rep.Findings, "focus")
	if len(focusItems) > 0 {
		fmt.Fprintln(w, headerStyle.Render("Files to review"))
		sortByPriority(focusItems)
		for _, f := range focusItems {
			icon := priorityIcon(f.RuleID)
			fmt.Fprintf(w, "  %s %s\n", icon, boldStyle.Render(f.File))
			fmt.Fprintf(w, "     %s\n", dimStyle.Render(f.Message))
		}
		fmt.Fprintln(w)
	}

	agentOrder := []string{"rules", "lint", "logic", "jira"}
	for _, agentName := range agentOrder {
		agentFindings := filterByAgent(rep.Findings, agentName)
		if len(agentFindings) == 0 {
			continue
		}
		fmt.Fprintln(w, headerStyle.Render(strings.ToUpper(agentName)+" agent"))
		sortBySeverity(agentFindings)
		for _, f := range agentFindings {
			icon, style := severityStyle(f.Severity, errorStyle, warningStyle, infoStyle)
			location := ""
			if loc := formatLocation(f); loc != "" {
				location = fileStyle.Render(loc) + " "
			}
			ruleLabel := ""
			if f.RuleID != "" {
				ruleLabel = dimStyle.Render("[" + f.RuleID + "] ")
			}
			fmt.Fprintf(w, "  %s %s%s%s\n", style.Render(icon), location, ruleLabel, f.Message)
			if f.Suggestion != "" {
				for _, line := range strings.Split(f.Suggestion, "\n") {
					fmt.Fprintf(w, "     %s\n", suggStyle.Render(line))
				}
			}
			fmt.Fprintln(w)
		}
	}
}

func (r *TerminalRenderer) renderPlain(rep *orchestrator.Report) {
	w := r.w
	pr := rep.PR
	fmt.Fprintf(w, "PR #%d: %s\n", pr.Number, pr.Title)
	fmt.Fprintf(w, "Branch: %s -> %s\n", pr.BranchName, pr.BaseSHA)

	errors, warnings, infos := countSeverities(rep.Findings)
	fmt.Fprintf(w, "Findings: %d errors, %d warnings, %d info\n\n", errors, warnings, infos)

	focusItems := filterByAgent(rep.Findings, "focus")
	if len(focusItems) > 0 {
		fmt.Fprintln(w, "=== FILES TO REVIEW ===")
		sortByPriority(focusItems)
		for _, f := range focusItems {
			fmt.Fprintf(w, "  %s\n    %s\n", f.File, f.Message)
		}
		fmt.Fprintln(w)
	}

	agentOrder := []string{"rules", "lint", "logic", "jira"}
	for _, agentName := range agentOrder {
		agentFindings := filterByAgent(rep.Findings, agentName)
		if len(agentFindings) == 0 {
			continue
		}
		fmt.Fprintf(w, "=== %s ===\n", strings.ToUpper(agentName))
		sortBySeverity(agentFindings)
		for _, f := range agentFindings {
			loc := ""
			if l := formatLocation(f); l != "" {
				loc = " " + l
			}
			rule := ""
			if f.RuleID != "" {
				rule = " [" + f.RuleID + "]"
			}
			fmt.Fprintf(w, "  [%s]%s%s %s\n", f.Severity, loc, rule, f.Message)
			if f.Suggestion != "" {
				fmt.Fprintf(w, "    Suggestion: %s\n", f.Suggestion)
			}
		}
		fmt.Fprintln(w)
	}
}

func severityStyle(sev agent.Severity, err, warn, info lipgloss.Style) (string, lipgloss.Style) {
	switch sev {
	case agent.SeverityError:
		return "✖", err
	case agent.SeverityWarning:
		return "⚠", warn
	default:
		return "ℹ", info
	}
}
