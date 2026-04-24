package agent

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	findingRe  = regexp.MustCompile(`(?s)<finding>(.*?)</finding>`)
	fileFocusRe = regexp.MustCompile(`(?s)<file_focus>(.*?)</file_focus>`)
	tagRe      = regexp.MustCompile(`(?s)<([a-z_]+)>(.*?)</([a-z_]+)>`)
)

// ParseFindings extracts Finding structs from an LLM XML response.
func ParseFindings(agentName, response string) []Finding {
	var findings []Finding
	matches := findingRe.FindAllStringSubmatch(response, -1)
	for _, m := range matches {
		block := m[1]
		sev := Severity(extractTag(block, "severity"))
		if sev == "" {
			sev = SeverityWarning
		}
		lineStart, _ := strconv.Atoi(extractTag(block, "line_start"))
		lineEnd, _ := strconv.Atoi(extractTag(block, "line_end"))
		findings = append(findings, Finding{
			Agent:      agentName,
			Severity:   sev,
			File:       extractTag(block, "file"),
			LineStart:  lineStart,
			LineEnd:    lineEnd,
			RuleID:     extractTag(block, "rule_id"),
			Message:    extractTag(block, "message"),
			Suggestion: extractTag(block, "suggestion"),
		})
	}
	return findings
}

// ParseFileFocus extracts FileFocus structs from a Focus agent LLM response.
func ParseFileFocus(response string) []FileFocus {
	var items []FileFocus
	matches := fileFocusRe.FindAllStringSubmatch(response, -1)
	for _, m := range matches {
		block := m[1]
		items = append(items, FileFocus{
			File:     extractTag(block, "file"),
			Priority: extractTag(block, "priority"),
			Reason:   extractTag(block, "reason"),
		})
	}
	return items
}

func extractTag(s, tag string) string {
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
