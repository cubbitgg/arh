package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/github"
)

// Report is the aggregated output of all agents.
type Report struct {
	PR       *github.PRData
	Findings []agent.Finding
}

// Orchestrator dispatches agents in parallel and aggregates their results.
type Orchestrator struct{}

// New creates an Orchestrator.
func New() *Orchestrator {
	return &Orchestrator{}
}

// Run executes all agents in parallel and returns the aggregated report.
// A failing agent does NOT abort others; its error becomes a synthetic Finding.
func (o *Orchestrator) Run(ctx context.Context, pr *github.PRData, agents []agent.Agent) (*Report, error) {
	type result struct {
		findings []agent.Finding
	}

	results := make([]result, len(agents))

	g, ctx := errgroup.WithContext(ctx)
	for i, a := range agents {
		i, a := i, a
		g.Go(func() error {
			start := time.Now()
			slog.Info("agent starting", "agent", a.Name())
			findings, err := a.Run(ctx, pr)
			elapsed := time.Since(start)
			if err != nil {
				slog.Warn("agent failed", "agent", a.Name(), "error", err, "duration", elapsed)
				results[i] = result{findings: []agent.Finding{
					{
						Agent:    a.Name(),
						Severity: agent.SeverityError,
						RuleID:   "agent-failure",
						Message:  fmt.Sprintf("%s agent failed: %v", a.Name(), err),
					},
				}}
				return nil // don't abort other agents
			}
			slog.Info("agent done", "agent", a.Name(), "findings", len(findings), "duration", elapsed)
			results[i] = result{findings: findings}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var allFindings []agent.Finding
	for _, r := range results {
		allFindings = append(allFindings, r.findings...)
	}

	return &Report{
		PR:       pr,
		Findings: deduplicate(allFindings),
	}, nil
}

// deduplicate removes findings with identical (Agent, File, LineStart, LineEnd, RuleID, Message).
func deduplicate(findings []agent.Finding) []agent.Finding {
	seen := make(map[string]bool)
	var out []agent.Finding
	for _, f := range findings {
		key := fmt.Sprintf("%s|%s|%d|%d|%s|%s", f.Agent, f.File, f.LineStart, f.LineEnd, f.RuleID, f.Message)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}
