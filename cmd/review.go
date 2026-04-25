package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	jiraclient "github.com/cubbitgg/arh/internal/jira"
	"github.com/cubbitgg/arh/internal/llm"
	"github.com/cubbitgg/arh/internal/orchestrator"
	"github.com/cubbitgg/arh/internal/report"
)

func init() {
	rootCmd.AddCommand(reviewCmd)

	reviewCmd.Flags().StringP("agents", "a", "", "comma-separated list of agents to run (default: all)")
	reviewCmd.Flags().StringP("output", "o", "", "output format: terminal, markdown, json, all (default: TUI if TTY, terminal if piped)")
	reviewCmd.Flags().Bool("no-tui", false, "disable interactive TUI; use static terminal output")
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

	renderers, err := buildRenderers(cmd, cfg)
	if err != nil {
		return fmt.Errorf("building renderers: %w", err)
	}

	// Run file-writing renderers first, TUI last (it blocks).
	var tuiRenderer report.Renderer
	for _, r := range renderers {
		if _, ok := r.(*report.TUIRenderer); ok {
			tuiRenderer = r
			continue
		}
		if err := r.Render(rep); err != nil {
			return fmt.Errorf("rendering report: %w", err)
		}
	}
	if tuiRenderer != nil {
		if err := tuiRenderer.Render(rep); err != nil {
			return fmt.Errorf("TUI: %w", err)
		}
	}

	return nil
}

func buildAgents(cfg *config.Config, filter string) ([]agent.Agent, error) {
	all := []string{"rules", "lint", "logic", "focus"}
	if cfg.Jira.Enabled {
		all = append(all, "jira")
	}
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

	if enabled["jira"] && cfg.Jira.Enabled {
		llmClient, err := llm.NewClient(cfg.LLMConfigFor("jira"))
		if err != nil {
			return nil, fmt.Errorf("creating LLM for jira agent: %w", err)
		}
		jc, err := buildJiraClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("creating Jira client: %w", err)
		}
		jiraAgent, err := agent.NewJiraAgent(cfg, llmClient, jc)
		if err != nil {
			return nil, fmt.Errorf("creating jira agent: %w", err)
		}
		agents = append(agents, jiraAgent)
	}

	return agents, nil
}

func buildJiraClient(cfg *config.Config) (jiraclient.JiraClient, error) {
	token := os.Getenv(cfg.Jira.APITokenEnv)
	email := os.Getenv(cfg.Jira.UserEmailEnv)
	return jiraclient.NewClient(cfg.Jira.BaseURL, email, token)
}

func buildRenderers(cmd *cobra.Command, cfg *config.Config) ([]report.Renderer, error) {
	outputFlag, _ := cmd.Flags().GetString("output")
	noTUI, _ := cmd.Flags().GetBool("no-tui")
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	var renderers []report.Renderer

	switch outputFlag {
	case "all":
		renderers = append(renderers, report.NewTerminalRenderer())
		mdPath := cfg.Output.MarkdownPath
		if mdPath == "" {
			mdPath = "./review-{pr_number}.md"
		}
		renderers = append(renderers, report.NewMarkdownRenderer(mdPath))
		jsonPath := cfg.Output.JSONPath
		if jsonPath == "" {
			jsonPath = "./review-{pr_number}.json"
		}
		renderers = append(renderers, report.NewJSONRenderer(jsonPath))

	case "markdown":
		mdPath := cfg.Output.MarkdownPath
		if mdPath == "" {
			mdPath = "./review-{pr_number}.md"
		}
		renderers = append(renderers, report.NewMarkdownRenderer(mdPath))

	case "json":
		jsonPath := cfg.Output.JSONPath
		if jsonPath == "" {
			jsonPath = "./review-{pr_number}.json"
		}
		renderers = append(renderers, report.NewJSONRenderer(jsonPath))

	case "terminal", "":
		if isTTY && !noTUI {
			mdPath := cfg.Output.MarkdownPath
			renderers = append(renderers, report.NewTUIRenderer(mdPath))
		} else {
			renderers = append(renderers, report.NewTerminalRenderer())
		}

	default:
		return nil, fmt.Errorf("unknown output format %q; valid: terminal, markdown, json, all", outputFlag)
	}

	// Also honour config-driven secondary outputs when not already included.
	if outputFlag != "all" && outputFlag != "markdown" && cfg.Output.Markdown && cfg.Output.MarkdownPath != "" {
		renderers = append(renderers, report.NewMarkdownRenderer(cfg.Output.MarkdownPath))
	}
	if outputFlag != "all" && outputFlag != "json" && cfg.Output.JSON && cfg.Output.JSONPath != "" {
		renderers = append(renderers, report.NewJSONRenderer(cfg.Output.JSONPath))
	}

	return renderers, nil
}
