package report

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// JSONRenderer writes the report as a JSON file for CI consumption.
type JSONRenderer struct {
	pathTemplate string
}

// NewJSONRenderer creates a renderer that writes to the resolved path.
func NewJSONRenderer(pathTemplate string) *JSONRenderer {
	return &JSONRenderer{pathTemplate: pathTemplate}
}

// Render writes the report to a JSON file.
func (r *JSONRenderer) Render(rep *orchestrator.Report) error {
	path := resolveOutputPath(r.pathTemplate, rep.PR.Number)

	pr := rep.PR
	errors, warnings, infos := countSeverities(rep.Findings)

	out := jsonReport{
		PR: jsonPR{
			Owner:   pr.Owner,
			Repo:    pr.Repo,
			Number:  pr.Number,
			Title:   pr.Title,
			Branch:  pr.BranchName,
			BaseSHA: pr.BaseSHA,
			HeadSHA: pr.HeadSHA,
		},
		Summary: jsonSummary{
			Errors:   errors,
			Warnings: warnings,
			Infos:    infos,
			Total:    errors + warnings + infos,
		},
		Findings: toJSONFindings(rep.Findings),
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing JSON report %s: %w", path, err)
	}

	return nil
}

type jsonReport struct {
	PR       jsonPR        `json:"pr"`
	Summary  jsonSummary   `json:"summary"`
	Findings []jsonFinding `json:"findings"`
}

type jsonPR struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Branch  string `json:"branch"`
	BaseSHA string `json:"base_sha"`
	HeadSHA string `json:"head_sha"`
}

type jsonSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
	Total    int `json:"total"`
}

type jsonFinding struct {
	Agent      string `json:"agent"`
	Severity   string `json:"severity"`
	File       string `json:"file,omitempty"`
	LineStart  int    `json:"line_start,omitempty"`
	LineEnd    int    `json:"line_end,omitempty"`
	RuleID     string `json:"rule_id,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func toJSONFindings(findings []agent.Finding) []jsonFinding {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, jsonFinding{
			Agent:      f.Agent,
			Severity:   string(f.Severity),
			File:       f.File,
			LineStart:  f.LineStart,
			LineEnd:    f.LineEnd,
			RuleID:     f.RuleID,
			Message:    f.Message,
			Suggestion: f.Suggestion,
		})
	}
	return out
}
