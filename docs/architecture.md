# Architecture

## Overview

arh is an orchestrator that dispatches independent analysis agents in parallel, collects their results, and renders a unified report.

```
arh review owner/repo#42
    │
    ▼
┌─────────────────────────────┐
│  Orchestrator               │
│  1. Fetch PR via gh CLI     │
│  2. Dispatch agents (||)    │
│  3. Aggregate findings      │
│  4. Render report           │
└─────────────────────────────┘
    │
    ├── Agent: Rules      (deterministic + LLM enrichment)
    ├── Agent: Lint        (golangci-lint + LLM enrichment)
    ├── Agent: Logic       (LLM per changed file)
    ├── Agent: Focus       (LLM on full diff summary)
    └── Agent: Jira        (Jira API + LLM validation)
```

Each agent has a single responsibility, receives only the data it needs (minimal context), and returns a standardized `[]Finding` slice. Agents do not communicate with each other.

## Project structure

```
arh/
├── cmd/
│   ├── main.go              # Entry point
│   ├── root.go              # Cobra root command
│   ├── review.go            # arh review subcommand
│   └── init.go              # arh init — generate config + agent prompts
├── internal/
│   ├── config/
│   │   └── config.go        # YAML config parsing and defaults
│   ├── github/
│   │   └── pr.go            # Fetch PR metadata + diff via gh CLI
│   ├── jira/
│   │   └── client.go        # Jira REST API v3 client
│   ├── llm/
│   │   ├── client.go        # LLMClient interface + factory
│   │   ├── anthropic.go     # Anthropic (Claude) implementation
│   │   ├── openai.go        # OpenAI implementation
│   │   └── ollama.go        # Ollama implementation (direct HTTP)
│   ├── agent/
│   │   ├── agent.go         # Agent interface, BaseAgent, prompt resolution
│   │   ├── finding.go       # Finding / FileFocus types
│   │   ├── parse.go         # XML response parser
│   │   ├── rules.go         # Rules agent
│   │   ├── lint.go          # Lint agent
│   │   ├── logic.go         # Logic agent
│   │   ├── focus.go         # Focus agent
│   │   ├── jira.go          # Jira agent
│   │   └── prompts/         # Built-in prompts (go:embed)
│   │       ├── rules.md
│   │       ├── lint.md
│   │       ├── logic.md
│   │       ├── focus.md
│   │       └── jira.md
│   ├── orchestrator/
│   │   └── orchestrator.go  # Parallel dispatch, aggregation, dedup
│   └── report/
│       ├── renderer.go      # Renderer interface + shared helpers
│       ├── terminal.go      # Colored terminal output (lipgloss)
│       ├── markdown.go      # Markdown file output
│       ├── json.go          # JSON file output (for CI)
│       └── tui.go           # Bubble Tea interactive TUI
├── docs/
├── .arh.yaml.example
├── CLAUDE.md
├── go.mod
└── go.sum
```

Built-in prompts live in `internal/agent/prompts/` and are embedded into the binary via `go:embed`. This keeps them editable during development while ensuring the binary is self-contained.

## Agent details

### Rules agent

- **Input**: PR metadata (branch name, commit messages, PR title, labels, description)
- **Deterministic pass**: regex validation on branch name pattern, conventional commits parsing, required labels check, PR title length, description presence. All rules defined in `.arh.yaml`.
- **LLM pass**: for each violation, the LLM proposes a corrected version. For rules that can't be checked deterministically (e.g. "PR title should summarize the change accurately"), the LLM handles the full check.
- **Output**: `[]Finding` with severity, message, and suggested fix.

### Lint agent

- **Input**: list of changed `.go` file paths
- **Deterministic pass**: runs `golangci-lint run --new-from-rev=<base_sha>` scoped to changed files. Parses JSON output.
- **LLM pass**: for each violation, the LLM explains why it matters in context and proposes a concrete code fix. Also checks custom codestyle rules defined in `.arh.yaml` that golangci-lint does not cover (naming conventions, comment style, etc.).
- **Output**: `[]Finding` with severity, file, line, message, and suggested fix.

### Logic agent

- **Input**: the diff of a single changed file + logic rules from `.arh.yaml`
- **Runs once per changed file** in parallel goroutines, bounded by `concurrency.logic_agent_parallelism` (default 4).
- **LLM only**: checks the diff against configured logic rules (error handling, test coverage, etc.).
- **Output**: `[]Finding` with severity, file, line range, message, and suggested fix.

### Focus agent

- **Input**: full diff summary (file list with change stats + condensed diff)
- **LLM only**: identifies which files contain meaningful logic changes vs boilerplate/config/docs. Ranks files by review priority (high/medium/low).
- **Output**: `[]FileFocus` converted to `[]Finding` with priority-based rule IDs.

### Jira agent

- **Input**: PR description text
- **Step 1**: extract Jira issue key from the PR description via configurable regex.
- **Step 2**: if found, fetch the issue from Jira REST API v3 (summary, description, acceptance criteria).
- **Step 3 (LLM)**: compare PR changes against acceptance criteria. Flags: unrelated work, uncovered AC, scope creep.
- **Graceful degradation**: empty description, missing key, Jira API errors, or LLM failures all produce warning findings instead of crashing.
- **Output**: `[]Finding` with Jira-specific context.

## Data model

All agents return the same struct:

```go
type Severity string
const (
    SeverityError   Severity = "error"    // Must fix before merge
    SeverityWarning Severity = "warning"  // Should review carefully
    SeverityInfo    Severity = "info"     // Suggestion / nice to have
)

type Finding struct {
    Agent       string   // "rules", "lint", "logic", "focus", "jira"
    Severity    Severity
    File        string   // empty for PR-level findings
    LineStart   int      // 0 if not applicable
    LineEnd     int      // 0 if not applicable
    Message     string   // human-readable description
    Suggestion  string   // proposed fix (code, commit msg, etc.)
    RuleID      string   // e.g. "no-swallowed-error", "missing-test"
}

type FileFocus struct {
    File     string
    Priority string // "high", "medium", "low"
    Reason   string
}
```

## Orchestrator

The orchestrator dispatches all agents in parallel using `errgroup`. A failing agent does **not** abort others — its error is captured as a synthetic `Finding` with rule ID `agent-failure`. After all agents complete, findings are deduplicated by `(Agent, File, LineStart, LineEnd, RuleID, Message)`.

## LLM client

All LLM interaction goes through a single interface:

```go
type LLMClient interface {
    Complete(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
}
```

Three implementations: Anthropic (official SDK), OpenAI (official SDK), Ollama (direct HTTP). The factory `NewClient` selects the implementation based on the `provider` field in config. Each agent gets its own client instance, enabling per-agent provider overrides.

## Report pipeline

The `Renderer` interface:

```go
type Renderer interface {
    Render(rep *orchestrator.Report) error
}
```

Four implementations:
- **TerminalRenderer** — auto-detects TTY for color vs plain output
- **MarkdownRenderer** — writes a `.md` file with sections per agent
- **JSONRenderer** — writes a structured `.json` for CI
- **TUIRenderer** — interactive Bubble Tea app with tabs, scrolling, detail view

Multiple renderers can be active simultaneously. File-writing renderers execute first; the TUI (which blocks) runs last.

## Build system

A `Makefile` at the repo root wraps common Go commands:

| Target | Description |
|--------|-------------|
| `make build` | Compile `arh` binary (output: `./arh`) |
| `make test` | Run all tests (`go test ./...`) |
| `make vet` | Static analysis (`go vet ./...`) |
| `make fmt` | Verify formatting with `gofmt` (non-zero exit if unformatted) |
| `make lint` | Run `golangci-lint` (requires it in `$PATH`) |
| `make check` | `fmt` + `vet` + `test` — full quality gate |
| `make clean` | Remove the compiled binary |
| `make install` | Install to `$GOPATH/bin` |

CI runs the `build-and-test` job (build + vet + test) and a separate `lint` job (fmt + golangci-lint) on every push to `main` and every pull request targeting `main`. See `.github/workflows/ci.yml`.

## Tech stack

| Component | Library |
|-----------|---------|
| CLI | `github.com/spf13/cobra` |
| TUI | `github.com/charmbracelet/bubbletea` + `bubbles` |
| Terminal styling | `github.com/charmbracelet/lipgloss` |
| Anthropic | `github.com/anthropics/anthropic-sdk-go` |
| OpenAI | `github.com/openai/openai-go` |
| Concurrency | `golang.org/x/sync/errgroup` |
| Config | `gopkg.in/yaml.v3` |
| GitHub | `gh` CLI (shelled out) |
| Jira | `net/http` (direct REST API v3) |
| Ollama | `net/http` (direct REST API) |
