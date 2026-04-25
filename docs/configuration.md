# Configuration Reference

arh is configured via `.arh.yaml`. The search order is:

1. Path given by `--config` flag
2. `.arh.yaml` in the current directory
3. `~/.config/arh/.arh.yaml`
4. Built-in defaults (no config file needed for basic usage)

Generate a starter config:

```bash
arh init
```

## Full config example

```yaml
# LLM provider configuration
llm:
  default:
    provider: anthropic          # anthropic | openai | ollama
    model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY  # env var name, NOT the key itself
  # Per-agent overrides (optional)
  overrides:
    rules:
      provider: ollama
      model: qwen2.5-coder:14b
      endpoint: http://localhost:11434
    focus:
      provider: openai
      model: gpt-4o
      api_key_env: OPENAI_API_KEY

# Concurrency
concurrency:
  logic_agent_parallelism: 4     # max parallel LLM calls for logic agent

# Rules agent
rules:
  branch_pattern: "^(feat|fix|chore|refactor|docs|test|ci)/[a-z0-9-]+$"
  require_conventional_commits: true
  required_labels:
    - needs-review
  pr_title_max_length: 72
  pr_description_required: true

# Lint agent
lint:
  golangci_lint_path: golangci-lint   # or absolute path
  extra_args: []                       # additional golangci-lint flags
  codestyle_rules:                     # LLM-checked rules (not covered by linter)
    - "Exported functions must have a godoc comment starting with the function name"
    - "Error variables must be named errXxx, not just err when multiple errors exist in scope"
    - "Context parameter must always be the first parameter"
    - "Use domain-specific names: say userID not id, orderTotal not total"

# Logic agent
logic:
  rules:
    - id: no-swallowed-error
      description: "Every if err != nil must return the error or log it, never silently ignore"
    - id: error-wrap-context
      description: "Error returns should wrap with fmt.Errorf describing the operation"
    - id: log-else-branch
      description: "Log statements in error paths should have a corresponding else or early return"
    - id: require-unit-test
      description: "New exported functions must have a corresponding _test.go test"
      severity: warning
    - id: require-e2e
      description: "New API endpoints or use cases should have a corresponding e2e test scenario"
      severity: warning

# Focus agent
focus:
  ignore_patterns:
    - "*.md"
    - "*.yaml"
    - "*.yml"
    - "go.sum"
    - "generated/**"

# Jira integration (optional)
jira:
  enabled: true
  base_url: https://mycompany.atlassian.net
  api_token_env: JIRA_API_TOKEN      # env var name
  user_email_env: JIRA_USER_EMAIL    # env var name
  issue_pattern: "[A-Z]{2,10}-\\d+"  # regex to extract issue key from PR description

# Output
output:
  terminal: true
  markdown: true
  markdown_path: ./review-{pr_number}.md
  json: true
  json_path: ./review-{pr_number}.json
```

## Section reference

### `llm`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default.provider` | string | `anthropic` | LLM provider: `anthropic`, `openai`, or `ollama` |
| `default.model` | string | `claude-sonnet-4-20250514` | Model name |
| `default.api_key_env` | string | `ANTHROPIC_API_KEY` | Env var containing the API key |
| `default.endpoint` | string | — | API endpoint (required for Ollama, e.g. `http://localhost:11434`) |
| `overrides.<agent>` | object | — | Per-agent override with same fields as `default` |

### `concurrency`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `logic_agent_parallelism` | int | `4` | Max concurrent LLM calls for the logic agent |

### `rules`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `branch_pattern` | string | — | Regex the branch name must match |
| `require_conventional_commits` | bool | `false` | Enforce Conventional Commits format |
| `required_labels` | []string | — | Labels that must be present on the PR |
| `pr_title_max_length` | int | `72` | Max PR title length |
| `pr_description_required` | bool | `false` | Require non-empty PR description |

### `lint`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `golangci_lint_path` | string | `golangci-lint` | Path to golangci-lint binary |
| `extra_args` | []string | `[]` | Additional golangci-lint flags |
| `codestyle_rules` | []string | — | Custom style rules checked by LLM (not golangci-lint) |

### `logic`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rules[].id` | string | — | Rule identifier (used in findings) |
| `rules[].description` | string | — | What the LLM should check |
| `rules[].severity` | string | `error` | Default severity: `error`, `warning`, or `info` |

### `focus`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ignore_patterns` | []string | — | Glob patterns for files the focus agent should deprioritize |

### `jira`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the Jira agent |
| `base_url` | string | — | Jira instance URL (e.g. `https://mycompany.atlassian.net`) |
| `api_token_env` | string | `JIRA_API_TOKEN` | Env var containing the Jira API token |
| `user_email_env` | string | `JIRA_USER_EMAIL` | Env var containing the Jira user email |
| `issue_pattern` | string | `[A-Z]{2,10}-\d+` | Regex to extract issue keys from the PR description |

### `output`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `terminal` | bool | `true` | Enable terminal output |
| `markdown` | bool | `false` | Also write a markdown file |
| `markdown_path` | string | `./review-{pr_number}.md` | Markdown output path (`{pr_number}` is replaced) |
| `json` | bool | `false` | Also write a JSON file |
| `json_path` | string | `./review-{pr_number}.json` | JSON output path |

## Environment variables

arh never stores API keys in config. It reads them from environment variables referenced in the config:

| Variable | Used by | Purpose |
|----------|---------|---------|
| `ANTHROPIC_API_KEY` | Anthropic LLM provider | Claude API key |
| `OPENAI_API_KEY` | OpenAI LLM provider | OpenAI API key |
| `JIRA_API_TOKEN` | Jira agent | Jira API token |
| `JIRA_USER_EMAIL` | Jira agent | Email for Jira Basic auth |

The env var names are configurable via `api_key_env`, `api_token_env`, and `user_email_env` fields.
