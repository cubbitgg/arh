package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
	"github.com/cubbitgg/arh/internal/orchestrator"
	"github.com/cubbitgg/arh/internal/report"
)

func init() {
	rootCmd.AddCommand(reviewCmd)

	reviewCmd.Flags().StringP("agents", "a", "", "comma-separated list of agents to run (default: all)")
	reviewCmd.Flags().StringP("output", "o", "terminal", "output format: terminal (default)")
	reviewCmd.Flags().Bool("no-tui", false, "disable interactive TUI (always on in this version)")
}

var reviewCmd = &cobra.Command{
	Use:   "review <owner/repo#N | github-pr-url>",
	Short: "Run pre-review triage on a GitHub PR",
	Args:  cobra.ExactArgs(1),
	RunE:  runReview,
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		fmt.Fprintln(os.Stderr, "Fetching PR data...")
	}

	pr, err := github.FetchPR(ctx, args[0])
	if err != nil {
		return fmt.Errorf("fetching PR: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Fetched PR #%d: %s (%d changed files)\n", pr.Number, pr.Title, len(pr.ChangedFiles))
	}

	agentFilter, _ := cmd.Flags().GetString("agents")
	agents, err := buildAgents(cfg, agentFilter)
	if err != nil {
		return fmt.Errorf("building agents: %w", err)
	}
	if len(agents) == 0 {
		return fmt.Errorf("no agents configured; check --agents flag and LLM config")
	}

	if verbose {
		names := make([]string, len(agents))
		for i, a := range agents {
			names[i] = a.Name()
		}
		fmt.Fprintf(os.Stderr, "Running agents: %s\n", strings.Join(names, ", "))
	}

	orch := orchestrator.New()
	rep, err := orch.Run(ctx, pr, agents)
	if err != nil {
		return fmt.Errorf("running orchestrator: %w", err)
	}

	renderer := report.NewTerminalRenderer()
	renderer.Render(os.Stdout, rep)

	return nil
}

func buildAgents(cfg *config.Config, filter string) ([]agent.Agent, error) {
	all := []string{"rules", "lint", "logic", "focus"}
	enabled := map[string]bool{}

	if filter == "" {
		for _, name := range all {
			enabled[name] = true
		}
	} else {
		for _, name := range strings.Split(filter, ",") {
			enabled[strings.TrimSpace(name)] = true
		}
	}

	var agents []agent.Agent

	if enabled["rules"] {
		llmClient, err := llm.NewClient(cfg.LLMConfigFor("rules"))
		if err != nil {
			return nil, fmt.Errorf("creating LLM for rules agent: %w", err)
		}
		agents = append(agents, agent.NewRulesAgent(cfg, llmClient))
	}

	if enabled["lint"] {
		llmClient, err := llm.NewClient(cfg.LLMConfigFor("lint"))
		if err != nil {
			return nil, fmt.Errorf("creating LLM for lint agent: %w", err)
		}
		agents = append(agents, agent.NewLintAgent(cfg, llmClient))
	}

	if enabled["logic"] {
		llmClient, err := llm.NewClient(cfg.LLMConfigFor("logic"))
		if err != nil {
			return nil, fmt.Errorf("creating LLM for logic agent: %w", err)
		}
		agents = append(agents, agent.NewLogicAgent(cfg, llmClient))
	}

	if enabled["focus"] {
		llmClient, err := llm.NewClient(cfg.LLMConfigFor("focus"))
		if err != nil {
			return nil, fmt.Errorf("creating LLM for focus agent: %w", err)
		}
		agents = append(agents, agent.NewFocusAgent(cfg, llmClient))
	}

	return agents, nil
}
