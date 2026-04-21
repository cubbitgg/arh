# arh (AI Review Helper)

## Project overview

`arh` is a local CLI tool that helps human reviewers prepare for GitHub PR reviews. It is NOT an automated reviewer that posts comments on PRs. It fetches a PR locally, runs multiple independent analysis agents, and produces a structured report telling the reviewer what to focus on.

The target user is a senior developer who receives a PR link and wants a pre-review triage before opening GitHub.

## Architecture

### Core principle: orchestrator + independent agents

The CLI is an orchestrator. It does not analyze code itself. It dispatches work to independent agents that run in parallel, collects their results, and aggregates them into a unified report.

Each agent has a single responsibility, receives only the data it needs (minimal context), and returns a standardized `[]Finding` slice. Agents do not communicate with each other.

```
arh review owner/repo#42
    │
    ▼
┌─────────────────────────────┐
│  Orchestrator (main loop)   │
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
