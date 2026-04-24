package report

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// TerminalRenderer renders a report to the terminal using lipgloss colors.
type TerminalRenderer struct {
	colorEnabled bool
}

// NewTerminalRenderer creates a renderer that auto-detects TTY.
func NewTerminalRenderer() *TerminalRenderer {
	return &TerminalRenderer{
		colorEnabled: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

// Render writes the full report to w.
func (r *TerminalRenderer) Render(w io.Writer, rep *orchestrator.Report) {
	if r.colorEnabled {
		r.renderColor(w, rep)
	} else {
		r.renderPlain(w, rep)
	}
}

func (r *TerminalRenderer) renderColor(w io.Writer, rep *orchestrator.Report) {
	boldStyle    := lipgloss.NewStyle().Bold(true)
	headerStyle  := lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("39"))
	errorStyle   := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	infoStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	fileStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	suggStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	dimStyle     := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	pr := rep.PR

	// Header
	fmt.Fprintln(w, headerStyle.Render(fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)))
	fmt.Fprintln(w, dimStyle.Render(fmt.Sprintf("  %s → %s", pr.BranchName, pr.BaseSHA[:8])))

	// Severity counts
	errors, warnings, infos := countSeverities(rep.Findings)
	fmt.Fprintf(w, "  %s  %s  %s\n\n",
		errorStyle.Render(fmt.Sprintf("✖ %d errors", errors)),
		warningStyle.Render(fmt.Sprintf("⚠ %d warnings", warnings)),
		infoStyle.Render(fmt.Sprintf("ℹ %d info", infos)),
	)

	// Focus section
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

	// Per-agent sections
	agentOrder := []string{"rules", "lint", "logic"}
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
			if f.File != "" {
				loc := f.File
				if f.LineStart > 0 {
					loc = fmt.Sprintf("%s:%d", f.File, f.LineStart)
					if f.LineEnd > f.LineStart {
						loc = fmt.Sprintf("%s-%d", loc, f.LineEnd)
					}
				}
				location = fileStyle.Render(loc) + " "
			}
			ruleLabel := ""
			if f.RuleID != "" {
				ruleLabel = dimStyle.Render("["+f.RuleID+"] ")
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

func (r *TerminalRenderer) renderPlain(w io.Writer, rep *orchestrator.Report) {
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

	agentOrder := []string{"rules", "lint", "logic"}
	for _, agentName := range agentOrder {
		agentFindings := filterByAgent(rep.Findings, agentName)
		if len(agentFindings) == 0 {
			continue
		}
		fmt.Fprintf(w, "=== %s ===\n", strings.ToUpper(agentName))
		sortBySeverity(agentFindings)
		for _, f := range agentFindings {
			loc := ""
			if f.File != "" {
				loc = f.File
				if f.LineStart > 0 {
					loc = fmt.Sprintf("%s:%d", loc, f.LineStart)
				}
				loc = " " + loc
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

func countSeverities(findings []agent.Finding) (errors, warnings, infos int) {
	for _, f := range findings {
		switch f.Severity {
		case agent.SeverityError:
			errors++
		case agent.SeverityWarning:
			warnings++
		case agent.SeverityInfo:
			infos++
		}
	}
	return
}

func filterByAgent(findings []agent.Finding, agentName string) []agent.Finding {
	var out []agent.Finding
	for _, f := range findings {
		if f.Agent == agentName {
			out = append(out, f)
		}
	}
	return out
}

func sortBySeverity(findings []agent.Finding) {
	order := map[agent.Severity]int{
		agent.SeverityError:   0,
		agent.SeverityWarning: 1,
		agent.SeverityInfo:    2,
	}
	sort.Slice(findings, func(i, j int) bool {
		oi, oj := order[findings[i].Severity], order[findings[j].Severity]
		if oi != oj {
			return oi < oj
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].LineStart < findings[j].LineStart
	})
}

func sortByPriority(findings []agent.Finding) {
	order := map[string]int{"focus-high": 0, "focus-medium": 1, "focus-low": 2}
	sort.Slice(findings, func(i, j int) bool {
		oi, oj := order[findings[i].RuleID], order[findings[j].RuleID]
		return oi < oj
	})
}

func priorityIcon(ruleID string) string {
	switch ruleID {
	case "focus-high":
		return "🔴"
	case "focus-medium":
		return "🟡"
	default:
		return "🟢"
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
