# arh — AI Review Helper

A CLI tool that triages GitHub PRs before you review them. It fetches a PR, runs parallel analysis agents (rules, lint, logic, focus, Jira), and produces a structured report telling you what to focus on.

`arh` is **not** an automated reviewer that posts comments. It's a local tool for human reviewers who want a head start.

```
arh review myorg/myrepo#42
```

```
PR #42: Add user authentication
  feat/user-auth → main
  ✖ 2 errors  ⚠ 4 warnings  ℹ 3 info

Files to review
  🔴 internal/auth/handler.go
     [high priority] Core auth logic with new middleware chain
  🟡 internal/auth/token.go
     [medium priority] JWT token generation — mostly boilerplate

RULES agent
  ⚠ [conventional-commits] Commit "added auth" does not follow format
     Suggestion: feat(auth): add JWT-based user authentication

LOGIC agent
  ✖ internal/auth/handler.go:45-52 [no-swallowed-error]
     Error from ValidateToken is silently ignored in the else branch
     Suggestion: return fmt.Errorf("validating token: %w", err)
```

## Requirements

- [gh CLI](https://cli.github.com/) — installed and authenticated (`gh auth login`)
- [golangci-lint](https://golangci-lint.run/) — for the lint agent
- At least one LLM provider: [Anthropic](https://console.anthropic.com/) API key, [OpenAI](https://platform.openai.com/) API key, or a local [Ollama](https://ollama.com/) instance

## Install

```bash
go install github.com/cubbitgg/arh/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/cubbitgg/arh.git
cd arh
go build -o arh ./cmd
```

## Quick start

```bash
# 1. Generate a config file
arh init

# 2. Set your LLM API key
export ANTHROPIC_API_KEY=sk-ant-...

# 3. Review a PR
arh review owner/repo#123
```

## Commands

```
arh review <owner/repo#N>     Review a PR (also accepts full GitHub URLs)
arh init                      Generate .arh.yaml in the current directory
arh init --agents             Also scaffold .arh/agents/ with built-in prompts
arh version                   Print version
```

### Flags for `arh review`

| Flag | Description |
|------|-------------|
| `-a, --agents` | Comma-separated list of agents to run (default: all). E.g. `--agents=rules,lint` |
| `-o, --output` | Output format: `terminal`, `markdown`, `json`, `all` |
| `--no-tui` | Disable interactive TUI, use static terminal output |
| `-c, --config` | Path to config file (default: `.arh.yaml`, then `~/.config/arh/.arh.yaml`) |
| `-v, --verbose` | Show agent execution details |

## Agents

| Agent | What it checks | How |
|-------|---------------|-----|
| **Rules** | Branch name, conventional commits, labels, PR title/description | Deterministic regex + LLM suggestions |
| **Lint** | Code style, golangci-lint violations, custom codestyle rules | golangci-lint + LLM explanations |
| **Logic** | Error handling, test coverage, code correctness (per file) | LLM analysis against configurable rules |
| **Focus** | Which files matter most for the reviewer | LLM prioritization of changes |
| **Jira** | PR alignment with linked Jira ticket | Jira API + LLM comparison |

## LLM providers

arh supports three providers. Each agent can use a different one:

```yaml
llm:
  default:
    provider: anthropic
    model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY
  overrides:
    rules:
      provider: ollama
      model: qwen2.5-coder:14b
      endpoint: http://localhost:11434
    focus:
      provider: openai
      model: gpt-4o
      api_key_env: OPENAI_API_KEY
```

| Provider | Config | Notes |
|----------|--------|-------|
| `anthropic` | `api_key_env` + `model` | Default. Uses Claude. |
| `openai` | `api_key_env` + `model` | GPT-4o, etc. |
| `ollama` | `endpoint` + `model` | Local, no API key needed. Must `ollama pull` the model first. |

## Output formats

| Mode | When | Description |
|------|------|-------------|
| **TUI** | Default in interactive terminals | Bubble Tea app with tabs, scrolling, detail view, keybindings |
| **Terminal** | When piped or `--no-tui` | Static colored output via lipgloss |
| **Markdown** | `--output=markdown` or config | Writes `review-{pr_number}.md` |
| **JSON** | `--output=json` or config | Writes `review-{pr_number}.json` for CI |

### TUI keybindings

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Switch agent tabs |
| `j`/`k` or arrows | Navigate findings |
| `enter` | Toggle detail panel |
| `o` | Open file in GitHub |
| `f` | Cycle severity filter |
| `m` | Export as markdown |
| `c` | Copy finding to clipboard |
| `q` | Quit |

## Customizing agent prompts

Each agent's LLM prompt can be fully overridden per-repo:

```bash
arh init --agents    # scaffolds .arh/agents/ with built-in prompts
```

Edit any file in `.arh/agents/` to change what that agent checks and how it reasons. Keep `{{output_format}}` in your override to ensure structured output parsing still works.

See [docs/prompt-customization.md](docs/prompt-customization.md) for details.

## Configuration

See [docs/configuration.md](docs/configuration.md) for the full config reference, or start with:

```bash
arh init
```

## Documentation

- [Architecture](docs/architecture.md) — how the orchestrator, agents, and report pipeline work
- [Configuration](docs/configuration.md) — full `.arh.yaml` reference
- [Prompt customization](docs/prompt-customization.md) — override agent prompts, template variables, design guidelines

## License

MIT
