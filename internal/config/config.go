package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type LLMProviderConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
	Endpoint  string `yaml:"endpoint"`
}

type LLMConfig struct {
	Default   LLMProviderConfig            `yaml:"default"`
	Overrides map[string]LLMProviderConfig `yaml:"overrides"`
}

type ConcurrencyConfig struct {
	LogicAgentParallelism int `yaml:"logic_agent_parallelism"`
}

type RuleConfig struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Severity    string `yaml:"severity"`
}

type RulesConfig struct {
	BranchPattern              string   `yaml:"branch_pattern"`
	RequireConventionalCommits bool     `yaml:"require_conventional_commits"`
	RequiredLabels             []string `yaml:"required_labels"`
	PRTitleMaxLength           int      `yaml:"pr_title_max_length"`
	PRDescriptionRequired      bool     `yaml:"pr_description_required"`
}

type LintConfig struct {
	GolangciLintPath string   `yaml:"golangci_lint_path"`
	ExtraArgs        []string `yaml:"extra_args"`
	CodestyleRules   []string `yaml:"codestyle_rules"`
}

type LogicConfig struct {
	Rules []RuleConfig `yaml:"rules"`
}

type FocusConfig struct {
	IgnorePatterns []string `yaml:"ignore_patterns"`
}

type JiraConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BaseURL      string `yaml:"base_url"`
	APITokenEnv  string `yaml:"api_token_env"`
	UserEmailEnv string `yaml:"user_email_env"`
	IssuePattern string `yaml:"issue_pattern"`
}

type OutputConfig struct {
	Terminal     bool   `yaml:"terminal"`
	Markdown     bool   `yaml:"markdown"`
	MarkdownPath string `yaml:"markdown_path"`
	JSON         bool   `yaml:"json"`
	JSONPath     string `yaml:"json_path"`
}

type Config struct {
	LLM         LLMConfig         `yaml:"llm"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	Rules       RulesConfig       `yaml:"rules"`
	Lint        LintConfig        `yaml:"lint"`
	Logic       LogicConfig       `yaml:"logic"`
	Focus       FocusConfig       `yaml:"focus"`
	Jira        JiraConfig        `yaml:"jira"`
	Output      OutputConfig      `yaml:"output"`
}

// Load reads the config from the first file that exists in the search path.
// path="" triggers auto-discovery.
func Load(path string) (*Config, error) {
	var paths []string
	if path != "" {
		paths = append(paths, path)
	}
	paths = append(paths, ".arh.yaml")
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "arh", ".arh.yaml"))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading config %s: %w", p, err)
		}
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", p, err)
		}
		applyDefaults(&cfg)
		return &cfg, nil
	}
	cfg := &Config{}
	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Concurrency.LogicAgentParallelism == 0 {
		cfg.Concurrency.LogicAgentParallelism = 4
	}
	if cfg.Lint.GolangciLintPath == "" {
		cfg.Lint.GolangciLintPath = "golangci-lint"
	}
	if cfg.Rules.PRTitleMaxLength == 0 {
		cfg.Rules.PRTitleMaxLength = 72
	}
	if cfg.LLM.Default.Provider == "" {
		cfg.LLM.Default.Provider = "anthropic"
	}
	if cfg.LLM.Default.Model == "" {
		cfg.LLM.Default.Model = "claude-sonnet-4-20250514"
	}
	if cfg.LLM.Default.APIKeyEnv == "" {
		cfg.LLM.Default.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	if cfg.Jira.IssuePattern == "" {
		cfg.Jira.IssuePattern = `[A-Z]{2,10}-\d+`
	}
	if cfg.Jira.APITokenEnv == "" {
		cfg.Jira.APITokenEnv = "JIRA_API_TOKEN"
	}
	if cfg.Jira.UserEmailEnv == "" {
		cfg.Jira.UserEmailEnv = "JIRA_USER_EMAIL"
	}
	if cfg.Output.Markdown && cfg.Output.MarkdownPath == "" {
		cfg.Output.MarkdownPath = "./review-{pr_number}.md"
	}
	if cfg.Output.JSON && cfg.Output.JSONPath == "" {
		cfg.Output.JSONPath = "./review-{pr_number}.json"
	}
}

// LLMConfigFor returns the LLM config for the given agent, applying overrides.
func (c *Config) LLMConfigFor(agentName string) LLMProviderConfig {
	if c.LLM.Overrides != nil {
		if override, ok := c.LLM.Overrides[agentName]; ok {
			return override
		}
	}
	return c.LLM.Default
}

// RulesForAgent returns the rules for the given agent as formatted plain text.
func (c *Config) RulesForAgent(agentName string) string {
	switch agentName {
	case "logic":
		var sb strings.Builder
		for _, r := range c.Logic.Rules {
			sev := r.Severity
			if sev == "" {
				sev = "error"
			}
			fmt.Fprintf(&sb, "- [%s] (%s) %s\n", r.ID, sev, r.Description)
		}
		return sb.String()
	case "rules":
		var sb strings.Builder
		if c.Rules.BranchPattern != "" {
			fmt.Fprintf(&sb, "- Branch name must match pattern: %s\n", c.Rules.BranchPattern)
		}
		if c.Rules.RequireConventionalCommits {
			sb.WriteString("- Commit messages must follow Conventional Commits format (type(scope): description)\n")
		}
		for _, lbl := range c.Rules.RequiredLabels {
			fmt.Fprintf(&sb, "- PR must have label: %s\n", lbl)
		}
		if c.Rules.PRTitleMaxLength > 0 {
			fmt.Fprintf(&sb, "- PR title must not exceed %d characters\n", c.Rules.PRTitleMaxLength)
		}
		if c.Rules.PRDescriptionRequired {
			sb.WriteString("- PR description must not be empty\n")
		}
		return sb.String()
	default:
		return ""
	}
}

// CodestyleRulesAsText returns the codestyle rules as newline-separated text.
func (c *Config) CodestyleRulesAsText() string {
	return strings.Join(c.Lint.CodestyleRules, "\n")
}

// IgnorePatternsAsText returns the focus ignore patterns as newline-separated text.
func (c *Config) IgnorePatternsAsText() string {
	return strings.Join(c.Focus.IgnorePatterns, ", ")
}
