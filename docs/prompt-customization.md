# Prompt Customization

Each LLM-based agent resolves its system prompt through a two-level mechanism. You can fully customize what agents check and how they reason without recompiling the binary.

## How it works

For each agent (e.g. `logic`), the prompt is resolved as:

1. **Check for override file** at `.arh/agents/logic.md`. If it exists, its content becomes the system prompt.
2. **Fall back to built-in** prompt embedded in the binary.
3. **Inject YAML variables** — template placeholders are replaced with values from `.arh.yaml`.

```
Agent "logic" starts
    │
    ├── .arh/agents/logic.md exists?
    │       ├── YES → use file content
    │       └── NO  → use built-in prompt
    │
    ▼
Inject {{rules}}, {{codestyle}}, {{output_format}}, ...
    │
    ▼
Send to LLM → parse structured output
```

## Getting started

Scaffold the override directory with built-in prompts as starting points:

```bash
arh init --agents
```

This creates:

```
.arh/
└── agents/
    ├── rules.md
    ├── lint.md
    ├── logic.md
    ├── focus.md
    └── jira.md
```

All files are optional. Delete any you don't want to customize — arh falls back to built-in prompts.

## Template variables

These placeholders can be used in any agent `.md` file:

| Placeholder | Source | Description |
|---|---|---|
| `{{rules}}` | `logic.rules[]` or `rules.*` in config | The agent's rules as formatted text |
| `{{codestyle}}` | `lint.codestyle_rules[]` | Codestyle rules not covered by linter |
| `{{severity_levels}}` | hardcoded | `"error, warning, info"` |
| `{{output_format}}` | hardcoded | XML format the Go parser expects |
| `{{ignore_patterns}}` | `focus.ignore_patterns[]` | Patterns for low-priority files |
| `{{language}}` | hardcoded | `"Go"` |
| `{{jira_issue_pattern}}` | `jira.issue_pattern` | Regex used to find Jira keys |

## Example override: `.arh/agents/logic.md`

```markdown
You are a Go code reviewer specializing in error handling
and observability for microservices.

Our stack uses zerolog for structured logging and gRPC for
inter-service communication. All errors must be wrapped with
the operation name using fmt.Errorf("operation: %w", err).

Check the following diff against these rules:
{{rules}}

For each violation, respond using this format:
{{output_format}}

Important constraints:
- Do NOT comment on naming or formatting — another agent handles that.
- Do NOT flag issues in test files unless the test itself has a bug.
- Every finding MUST include a concrete code suggestion, not just a description.
- Classify severity as: error (must fix), warning (should review), info (nice to have).
```

## Key constraint

The override file controls the **prompt** (how the LLM thinks), but **not** the output contract. The `{{output_format}}` placeholder ensures the LLM always responds in the structured XML format the Go parser expects.

This means:
- You can change what the agent checks, how it reasons, and what context it has.
- You cannot break the Finding parsing pipeline — the output schema is enforced.
- The YAML rules are always available via `{{rules}}` even in override files, so you don't duplicate them.

**Never remove `{{output_format}}` from an override file** — without it, the LLM response won't be parseable.

## Prompt design guidelines

When writing or editing agent prompts:

1. **Be specific about the role**: "You are a Go code reviewer checking error handling patterns."
2. **Use `{{rules}}`**: don't duplicate rules from YAML — reference them via the template variable.
3. **Always include `{{output_format}}`**: ensures parseable output.
4. **Include examples**: show one good finding and one `<no_findings/>` case to calibrate.
5. **Constrain scope**: explicitly tell the LLM what NOT to review (e.g. "Do not comment on naming — another agent handles that").
6. **Request line numbers**: always ask for specific line ranges from the diff.
7. **Ask for severity**: have the LLM classify each finding as error/warning/info.
8. **Ask for a fix**: every finding should include a concrete suggestion, not just a description.

## Sharing prompts across teams

Since `.arh/agents/*.md` files are plain text, teams can:
- Version control prompts alongside the codebase.
- Have different configs per repo (useful in monorepos).
- Share and fork agent prompts like any other config file.
