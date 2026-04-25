package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cubbitgg/arh/internal/agent"
)

const defaultConfig = `# arh configuration file
# See https://github.com/cubbitgg/arh for documentation

llm:
  default:
    provider: anthropic          # anthropic | openai | ollama
    model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY
  # Per-agent overrides (optional)
  # overrides:
  #   rules:
  #     provider: ollama
  #     model: qwen2.5-coder:14b
  #     endpoint: http://localhost:11434
  #   focus:
  #     provider: openai
  #     model: gpt-4o
  #     api_key_env: OPENAI_API_KEY

concurrency:
  logic_agent_parallelism: 4

rules:
  branch_pattern: "^(feat|fix|chore|refactor|docs|test|ci)/[a-z0-9-]+$"
  require_conventional_commits: true
  pr_title_max_length: 72
  pr_description_required: true

lint:
  golangci_lint_path: golangci-lint
  codestyle_rules:
    - "Exported functions must have a godoc comment starting with the function name"
    - "Context parameter must always be the first parameter"
    - "Use domain-specific names: say userID not id, orderTotal not total"

logic:
  rules:
    - id: no-swallowed-error
      description: "Every if err != nil must return the error or log it, never silently ignore"
    - id: error-wrap-context
      description: "Error returns should wrap with fmt.Errorf describing the operation: fmt.Errorf(\"doing X: %w\", err)"
    - id: require-unit-test
      description: "New exported functions must have a corresponding _test.go test"
      severity: warning
    - id: require-e2e
      description: "New API endpoints or use cases should have a corresponding e2e test scenario"
      severity: warning

focus:
  ignore_patterns:
    - "*.md"
    - "*.yaml"
    - "*.yml"
    - "go.sum"
    - "generated/**"

# Jira integration (optional)
# jira:
#   enabled: true
#   base_url: https://mycompany.atlassian.net
#   api_token_env: JIRA_API_TOKEN
#   user_email_env: JIRA_USER_EMAIL
#   issue_pattern: "[A-Z]{2,10}-\\d+"

output:
  terminal: true
  # markdown: true
  # markdown_path: ./review-{pr_number}.md
  # json: true
  # json_path: ./review-{pr_number}.json
`

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().Bool("agents", false, "also scaffold .arh/agents/ directory with built-in prompt files")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a default .arh.yaml in the current directory",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	const target = ".arh.yaml"
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists; remove it first to regenerate", target)
	}
	if err := os.WriteFile(target, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	fmt.Fprintf(os.Stdout, "Created %s — edit it to match your project's conventions.\n", target)
	fmt.Fprintln(os.Stdout, "Set ANTHROPIC_API_KEY and run: arh review <owner/repo#N>")

	scaffoldAgents, _ := cmd.Flags().GetBool("agents")
	if scaffoldAgents {
		if err := writeAgentPrompts(); err != nil {
			return err
		}
	}

	return nil
}

func writeAgentPrompts() error {
	dir := filepath.Join(".arh", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	prompts := agent.BuiltinPrompts()

	// Sort names for deterministic output.
	names := make([]string, 0, len(prompts))
	for name := range prompts {
		names = append(names, name)
	}
	sort.Strings(names)

	var skipped []string
	for _, name := range names {
		target := filepath.Join(dir, name+".md")
		if _, err := os.Stat(target); err == nil {
			skipped = append(skipped, target)
			continue
		}
		if err := os.WriteFile(target, []byte(prompts[name]), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		fmt.Fprintf(os.Stdout, "Created %s\n", target)
	}

	if len(skipped) > 0 {
		fmt.Fprintf(os.Stderr, "Skipped (already exist): %s\n", strings.Join(skipped, ", "))
	}
	fmt.Fprintln(os.Stdout, "Customize these files to tailor agent behavior — keep {{output_format}} to preserve structured output.")

	return nil
}
