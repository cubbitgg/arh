package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const defaultConfig = `# arh configuration file
# See https://github.com/cubbitgg/arh for documentation

llm:
  default:
    provider: anthropic
    model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY

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

output:
  terminal: true
`

func init() {
	rootCmd.AddCommand(initCmd)
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
	return nil
}
