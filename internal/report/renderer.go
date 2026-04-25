package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// Renderer writes a report to some output destination.
type Renderer interface {
	Render(rep *orchestrator.Report) error
}

// resolveOutputPath replaces {pr_number} in the template with the actual PR number.
func resolveOutputPath(template string, prNumber int) string {
	return strings.ReplaceAll(template, "{pr_number}", fmt.Sprintf("%d", prNumber))
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

func formatLocation(f agent.Finding) string {
	if f.File == "" {
		return ""
	}
	loc := f.File
	if f.LineStart > 0 {
		loc = fmt.Sprintf("%s:%d", loc, f.LineStart)
		if f.LineEnd > f.LineStart {
			loc = fmt.Sprintf("%s-%d", loc, f.LineEnd)
		}
	}
	return loc
}
