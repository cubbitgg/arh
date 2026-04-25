package agent

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubbitgg/arh/internal/config"
	"github.com/cubbitgg/arh/internal/github"
	"github.com/cubbitgg/arh/internal/llm"
)

//go:embed prompts/rules.md
var rulesBuiltinPrompt string

//go:embed prompts/lint.md
var lintBuiltinPrompt string

//go:embed prompts/logic.md
var logicBuiltinPrompt string

//go:embed prompts/focus.md
var focusBuiltinPrompt string

//go:embed prompts/jira.md
var jiraBuiltinPrompt string

// Agent analyzes a PR and returns findings.
type Agent interface {
	Name() string
	Run(ctx context.Context, pr *github.PRData) ([]Finding, error)
}

// findingOutputFormat is the XML contract for all standard agents.
const findingOutputFormat = `For each issue found, respond with one or more XML blocks in this exact format:
<finding>
  <severity>error|warning|info</severity>
  <file>path/to/file.go</file>
  <line_start>42</line_start>
  <line_end>50</line_end>
  <rule_id>rule-id-here</rule_id>
  <message>Clear description of the issue</message>
  <suggestion>Concrete fix or corrected version</suggestion>
</finding>

If there are no issues, respond with: <no_findings/>`

// fileFocusOutputFormat is the XML contract for the Focus agent.
const fileFocusOutputFormat = `For each file, respond with one XML block in this exact format:
<file_focus>
  <file>path/to/file.go</file>
  <priority>high|medium|low</priority>
  <reason>One sentence explaining why a reviewer should (or need not) focus here</reason>
</file_focus>`

// BaseAgent provides shared prompt resolution and LLM client creation for all agents.
type BaseAgent struct {
	name          string
	builtinPrompt string
	outputFmt     string
	cfg           *config.Config
	llm           llm.LLMClient
}

// newBaseAgent constructs a BaseAgent. Pass findingOutputFormat or fileFocusOutputFormat for outputFmt.
func newBaseAgent(name, builtinPrompt, outputFmt string, cfg *config.Config, llmClient llm.LLMClient) BaseAgent {
	return BaseAgent{
		name:          name,
		builtinPrompt: builtinPrompt,
		outputFmt:     outputFmt,
		cfg:           cfg,
		llm:           llmClient,
	}
}

// Name returns the agent's identifier string.
func (b *BaseAgent) Name() string { return b.name }

// resolvePrompt returns the system prompt, applying override file and YAML variable injection.
func (b *BaseAgent) resolvePrompt() string {
	var tmpl string

	// Check for user override in .arh/agents/<name>.md
	overridePath := filepath.Join(".arh", "agents", b.name+".md")
	if content, err := os.ReadFile(overridePath); err == nil {
		tmpl = string(content)
	} else {
		tmpl = b.builtinPrompt
	}

	return b.injectVariables(tmpl)
}

// injectVariables replaces template placeholders with values from config.
func (b *BaseAgent) injectVariables(tmpl string) string {
	r := strings.NewReplacer(
		"{{rules}}", b.cfg.RulesForAgent(b.name),
		"{{codestyle}}", b.cfg.CodestyleRulesAsText(),
		"{{severity_levels}}", "error, warning, info",
		"{{output_format}}", b.outputFmt,
		"{{ignore_patterns}}", b.cfg.IgnorePatternsAsText(),
		"{{language}}", "Go",
		"{{jira_issue_pattern}}", b.cfg.Jira.IssuePattern,
	)
	return r.Replace(tmpl)
}

// BuiltinPrompts returns all built-in agent prompt contents keyed by agent name.
// Used by the init command to scaffold .arh/agents/ override files.
func BuiltinPrompts() map[string]string {
	return map[string]string{
		"rules": rulesBuiltinPrompt,
		"lint":  lintBuiltinPrompt,
		"logic": logicBuiltinPrompt,
		"focus": focusBuiltinPrompt,
		"jira":  jiraBuiltinPrompt,
	}
}

// completeLLM resolves the prompt and sends a request to the configured LLM.
func (b *BaseAgent) completeLLM(ctx context.Context, userPrompt string) (string, error) {
	systemPrompt := b.resolvePrompt()
	resp, err := b.llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("%s agent LLM call: %w", b.name, err)
	}
	return resp, nil
}
