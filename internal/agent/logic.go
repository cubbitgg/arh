package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
)

// LogicAgent reviews each changed Go file for logic correctness via LLM.
type LogicAgent struct {
	BaseAgent
}

// NewLogicAgent creates a Logic agent.
func NewLogicAgent(cfg *config.Config, llmClient llm.LLMClient) *LogicAgent {
	return &LogicAgent{
		BaseAgent: newBaseAgent("logic", logicBuiltinPrompt, findingOutputFormat, cfg, llmClient),
	}
}

// Run analyzes each changed Go file in parallel, bounded by the configured concurrency limit.
func (a *LogicAgent) Run(ctx context.Context, pr *github.PRData) ([]Finding, error) {
	goFiles := filterGoFiles(pr.ChangedFiles)
	if len(goFiles) == 0 {
		return nil, nil
	}

	parallelism := a.cfg.Concurrency.LogicAgentParallelism
	if parallelism <= 0 {
		parallelism = 4
	}

	sem := make(chan struct{}, parallelism)
	var mu sync.Mutex
	var findings []Finding
	var wg sync.WaitGroup

	for _, f := range goFiles {
		f := f
		diff := pr.PerFileDiff[f.Path]
		if diff == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := a.analyzeFile(ctx, f.Path, diff)
			if err != nil {
				mu.Lock()
				findings = append(findings, Finding{
					Agent:    "logic",
					Severity: SeverityWarning,
					File:     f.Path,
					RuleID:   "logic-agent-error",
					Message:  fmt.Sprintf("logic agent failed for %s: %v", f.Path, err),
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			findings = append(findings, result...)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return findings, nil
}

func (a *LogicAgent) analyzeFile(ctx context.Context, filePath, diff string) ([]Finding, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "File: %s\n\n", filePath)
	fmt.Fprintf(&sb, "Diff:\n%s\n", truncate(diff, 3000))

	resp, err := a.completeLLM(ctx, sb.String())
	if err != nil {
		return nil, err
	}
	findings := ParseFindings("logic", resp)
	// Ensure file is set
	for i := range findings {
		if findings[i].File == "" {
			findings[i].File = filePath
		}
	}
	return findings, nil
}
