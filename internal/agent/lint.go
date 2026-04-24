package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
)

// LintAgent runs golangci-lint and enriches violations via LLM.
type LintAgent struct {
	BaseAgent
}

// NewLintAgent creates a Lint agent.
func NewLintAgent(cfg *config.Config, llmClient llm.LLMClient) *LintAgent {
	return &LintAgent{
		BaseAgent: newBaseAgent("lint", lintBuiltinPrompt, findingOutputFormat, cfg, llmClient),
	}
}

type golangciOutput struct {
	Issues []golangciIssue `json:"Issues"`
}

type golangciIssue struct {
	FromLinter string `json:"FromLinter"`
	Text       string `json:"Text"`
	Pos        struct {
		Filename string `json:"Filename"`
		Line     int    `json:"Line"`
		Column   int    `json:"Column"`
	} `json:"Pos"`
	LineRange *struct {
		From int `json:"From"`
		To   int `json:"To"`
	} `json:"LineRange"`
	SourceLines []string `json:"SourceLines"`
}

// Run executes golangci-lint and then enriches each violation with LLM context.
func (a *LintAgent) Run(ctx context.Context, pr *github.PRData) ([]Finding, error) {
	goFiles := filterGoFiles(pr.ChangedFiles)
	if len(goFiles) == 0 {
		return nil, nil
	}

	issues, lintErr := a.runGolangciLint(ctx, pr.BaseSHA, goFiles)
	var findings []Finding

	if lintErr != nil {
		// Golangci-lint failure is a non-fatal warning
		findings = append(findings, Finding{
			Agent:    "lint",
			Severity: SeverityWarning,
			RuleID:   "lint-unavailable",
			Message:  fmt.Sprintf("golangci-lint could not run: %v", lintErr),
		})
	} else {
		// Enrich each violation with LLM
		for _, issue := range issues {
			enriched, err := a.enrichIssue(ctx, issue, pr)
			if err != nil {
				// Fall back to a basic finding
				sev := SeverityWarning
				if issue.FromLinter == "errcheck" || issue.FromLinter == "staticcheck" {
					sev = SeverityError
				}
				findings = append(findings, Finding{
					Agent:     "lint",
					Severity:  sev,
					File:      issue.Pos.Filename,
					LineStart: issue.Pos.Line,
					LineEnd:   issue.Pos.Line,
					RuleID:    issue.FromLinter,
					Message:   issue.Text,
				})
			} else {
				findings = append(findings, enriched...)
			}
		}
	}

	// Additional codestyle pass for rules golangci-lint doesn't cover
	if len(a.cfg.Lint.CodestyleRules) > 0 {
		styleFindings, err := a.codestylePass(ctx, goFiles, pr)
		if err == nil {
			findings = append(findings, styleFindings...)
		}
	}

	return findings, nil
}

func (a *LintAgent) runGolangciLint(ctx context.Context, baseSHA string, goFiles []github.ChangedFile) ([]golangciIssue, error) {
	lintPath := a.cfg.Lint.GolangciLintPath
	args := []string{"run", "--out-format=json"}
	if baseSHA != "" {
		args = append(args, "--new-from-rev="+baseSHA)
	}
	args = append(args, a.cfg.Lint.ExtraArgs...)
	args = append(args, "./...")

	cmd := exec.CommandContext(ctx, lintPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// golangci-lint exits with code 1 when it finds issues — that's normal
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("golangci-lint produced no output: %s", stderr.String())
	}

	var out golangciOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("parsing golangci-lint output: %w", err)
	}

	// Filter to only changed files
	changedSet := make(map[string]bool)
	for _, f := range goFiles {
		changedSet[f.Path] = true
		// Also try just the base filename for relative path matching
		changedSet[filepath.Base(f.Path)] = true
	}

	var filtered []golangciIssue
	for _, issue := range out.Issues {
		if changedSet[issue.Pos.Filename] || changedSet[filepath.Base(issue.Pos.Filename)] {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

func (a *LintAgent) enrichIssue(ctx context.Context, issue golangciIssue, pr *github.PRData) ([]Finding, error) {
	snippet := strings.Join(issue.SourceLines, "\n")
	fileDiff := pr.PerFileDiff[issue.Pos.Filename]

	userPrompt := fmt.Sprintf(
		"File: %s\nLine: %d\nLinter: %s\nViolation: %s\n\nSource code at violation:\n%s\n\nFile diff context:\n%s\n\nExplain why this matters in context and provide a concrete code fix.",
		issue.Pos.Filename, issue.Pos.Line, issue.FromLinter, issue.Text, snippet, truncate(fileDiff, 1500),
	)

	resp, err := a.completeLLM(ctx, userPrompt)
	if err != nil {
		return nil, err
	}
	findings := ParseFindings("lint", resp)
	// Ensure file/line are set (LLM might omit them)
	for i := range findings {
		if findings[i].File == "" {
			findings[i].File = issue.Pos.Filename
		}
		if findings[i].LineStart == 0 {
			findings[i].LineStart = issue.Pos.Line
		}
	}
	return findings, nil
}

func (a *LintAgent) codestylePass(ctx context.Context, goFiles []github.ChangedFile, pr *github.PRData) ([]Finding, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Check the following Go file diffs against these codestyle rules:\n%s\n\n", a.cfg.CodestyleRulesAsText())
	for _, f := range goFiles {
		diff := pr.PerFileDiff[f.Path]
		if diff == "" {
			continue
		}
		fmt.Fprintf(&sb, "=== %s ===\n%s\n\n", f.Path, truncate(diff, 2000))
	}

	resp, err := a.completeLLM(ctx, sb.String())
	if err != nil {
		return nil, err
	}
	return ParseFindings("lint", resp), nil
}

func filterGoFiles(files []github.ChangedFile) []github.ChangedFile {
	var result []github.ChangedFile
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".go") && f.Status != "removed" {
			result = append(result, f)
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
