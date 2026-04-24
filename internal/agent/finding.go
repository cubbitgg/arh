package agent

// Severity represents how critical a finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding is a single issue identified by an agent.
type Finding struct {
	Agent      string
	Severity   Severity
	File       string
	LineStart  int
	LineEnd    int
	Message    string
	Suggestion string
	RuleID     string
}

// FileFocus is a priority classification of a changed file by the Focus agent.
type FileFocus struct {
	File     string
	Priority string // "high", "medium", "low"
	Reason   string
}
