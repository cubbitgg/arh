# arh — AI Review Helper

[![CI](https://github.com/cubbitgg/arh/actions/workflows/ci.yml/badge.svg)](https://github.com/cubbitgg/arh/actions/workflows/ci.yml)
[![Build](https://github.com/cubbitgg/arh/actions/workflows/ci.yml/badge.svg?event=push&label=build)](https://github.com/cubbitgg/arh/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/cubbitgg/arh?logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/cubbitgg/arh?logo=github)](https://github.com/cubbitgg/arh/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/cubbitgg/arh)](https://goreportcard.com/report/github.com/cubbitgg/arh)

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

## ✨ Features

- **Five parallel agents** — Rules, Lint, Logic, Focus, and Jira run concurrently; a failing agent never blocks the others
- **Three LLM providers** — Anthropic (Claude), OpenAI, and Ollama; each agent can use a different provider
- **Deterministic + LLM hybrid** — branch pattern, conventional commits, and lint checks run locally; LLM only enriches and explains
- **Interactive TUI** — Bubble Tea app with tabs per agent, scrollable findings, detail panel, severity filter, and one-key markdown export
- **Multiple output formats** — terminal (static), interactive TUI, Markdown file, and JSON (CI-friendly)
- **Customizable agent prompts** — override any agent's system prompt per repository without recompiling
- **Jira integration** — extracts the linked issue key, fetches acceptance criteria, and flags scope creep or uncovered AC
- **Zero comment posting** — reads PRs, never writes to them

## ⚙️ What you can configure

All behaviour is driven by `.arh.yaml`. Below is a summary — see [docs/configuration.md](docs/configuration.md) for the full reference.

| Area | Configurable |
|------|-------------|
| **LLM** | Provider (`anthropic` / `openai` / `ollama`), model, API key env var, endpoint; per-agent overrides |
| **Rules** | Branch name regex, conventional commits enforcement, required PR labels, title max length, description requirement |
| **Lint** | Path to `golangci-lint`, extra flags, custom codestyle rules checked by LLM |
| **Logic** | Custom rules with ID, description, and severity (`error` / `warning` / `info`) |
| **Focus** | Glob patterns for files to deprioritize (e.g. `*.md`, `generated/**`) |
| **Jira** | Base URL, API token, user email, issue key regex |
| **Output** | Enable/disable terminal, markdown, JSON; file path templates with `{pr_number}` |
| **Concurrency** | Max parallel LLM calls for the logic agent |
| **Agent prompts** | Full system-prompt override per agent via `.arh/agents/<name>.md` — see [docs/prompt-customization.md](docs/prompt-customization.md) |

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
make build
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

Run `arh review --help` for the full list of flags.

## Agents

| Agent | What it checks | How |
|-------|---------------|-----|
| **Rules** | Branch name, conventional commits, labels, PR title/description | Deterministic regex + LLM suggestions |
| **Lint** | Code style, golangci-lint violations, custom codestyle rules | golangci-lint + LLM explanations |
| **Logic** | Error handling, test coverage, code correctness (per file) | LLM analysis against configurable rules |
| **Focus** | Which files matter most for the reviewer | LLM prioritization of changes |
| **Jira** | PR alignment with linked Jira ticket | Jira API + LLM comparison |

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

## Documentation

- [Architecture](docs/architecture.md) — orchestrator, agents, report pipeline, build system
- [Configuration](docs/configuration.md) — full `.arh.yaml` reference with all fields, types, and defaults
- [Prompt customization](docs/prompt-customization.md) — override agent prompts, template variables, design guidelines

## License

MIT
