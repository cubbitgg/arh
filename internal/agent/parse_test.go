package agent

import (
	"testing"
)

func TestParseFindings(t *testing.T) {
	input := `
<finding>
  <severity>error</severity>
  <file>main.go</file>
  <line_start>10</line_start>
  <line_end>12</line_end>
  <rule_id>no-swallowed-error</rule_id>
  <message>error is discarded</message>
  <suggestion>return the error</suggestion>
</finding>
<finding>
  <file>util.go</file>
  <rule_id>missing-test</rule_id>
  <message>no test coverage</message>
</finding>`

	findings := ParseFindings("logic", input)

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	f := findings[0]
	if f.Agent != "logic" {
		t.Errorf("agent: got %q, want %q", f.Agent, "logic")
	}
	if f.Severity != SeverityError {
		t.Errorf("severity: got %q, want %q", f.Severity, SeverityError)
	}
	if f.File != "main.go" {
		t.Errorf("file: got %q, want %q", f.File, "main.go")
	}
	if f.LineStart != 10 {
		t.Errorf("line_start: got %d, want 10", f.LineStart)
	}
	if f.LineEnd != 12 {
		t.Errorf("line_end: got %d, want 12", f.LineEnd)
	}
	if f.RuleID != "no-swallowed-error" {
		t.Errorf("rule_id: got %q, want %q", f.RuleID, "no-swallowed-error")
	}

	// second finding has no severity — should default to warning
	if findings[1].Severity != SeverityWarning {
		t.Errorf("default severity: got %q, want %q", findings[1].Severity, SeverityWarning)
	}
}

func TestParseFindings_empty(t *testing.T) {
	if findings := ParseFindings("rules", "no xml here"); findings != nil {
		t.Errorf("expected nil, got %v", findings)
	}
}

func TestParseFileFocus(t *testing.T) {
	input := `
<file_focus>
  <file>internal/agent/agent.go</file>
  <priority>high</priority>
  <reason>core logic changed</reason>
</file_focus>
<file_focus>
  <file>README.md</file>
  <priority>low</priority>
  <reason>docs only</reason>
</file_focus>`

	items := ParseFileFocus(input)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].File != "internal/agent/agent.go" {
		t.Errorf("file: got %q", items[0].File)
	}
	if items[0].Priority != "high" {
		t.Errorf("priority: got %q", items[0].Priority)
	}
	if items[1].Priority != "low" {
		t.Errorf("priority: got %q", items[1].Priority)
	}
}
