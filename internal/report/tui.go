package report

import (
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cubbitgg/arh/internal/agent"
	"github.com/cubbitgg/arh/internal/orchestrator"
)

// TUIRenderer runs an interactive Bubble Tea TUI for the report.
type TUIRenderer struct {
	markdownPath string
}

// NewTUIRenderer creates a TUI renderer. markdownPath is used for the "m" export keybinding.
func NewTUIRenderer(markdownPath string) *TUIRenderer {
	return &TUIRenderer{markdownPath: markdownPath}
}

// Render starts the interactive TUI. It blocks until the user quits.
func (r *TUIRenderer) Render(rep *orchestrator.Report) error {
	m := newTUIModel(rep, r.markdownPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ----- model -----

type statusMsg string

type tuiModel struct {
	report       *orchestrator.Report
	markdownPath string

	agents         []string
	activeTab      int
	cursor         int
	detailOpen     bool
	severityFilter agent.Severity

	agentFindings map[string][]agent.Finding

	viewport viewport.Model
	width    int
	height   int

	statusMsg string
	keys      keyMap
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Enter    key.Binding
	Open     key.Binding
	Filter   key.Binding
	Export   key.Binding
	Copy     key.Binding
	Quit     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next agent")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev agent")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "toggle detail")),
		Open:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in GitHub")),
		Filter:   key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter severity")),
		Export:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "export markdown")),
		Copy:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy finding")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func newTUIModel(rep *orchestrator.Report, markdownPath string) tuiModel {
	// Determine ordered agent list from findings.
	agentOrder := []string{"focus", "rules", "lint", "logic", "jira"}
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		seen[f.Agent] = true
	}
	var agents []string
	for _, a := range agentOrder {
		if seen[a] {
			agents = append(agents, a)
		}
	}
	// Any agent not in the canonical order goes at the end.
	for _, f := range rep.Findings {
		if !slices.Contains(agentOrder, f.Agent) && !seen[f.Agent+"-processed"] {
			agents = append(agents, f.Agent)
			seen[f.Agent+"-processed"] = true
		}
	}

	agentFindings := make(map[string][]agent.Finding)
	for _, a := range agents {
		findings := filterByAgent(rep.Findings, a)
		sortBySeverity(findings)
		if a == "focus" {
			sortByPriority(findings)
		}
		agentFindings[a] = findings
	}

	return tuiModel{
		report:        rep,
		markdownPath:  markdownPath,
		agents:        agents,
		agentFindings: agentFindings,
		viewport:      viewport.New(80, 20),
		keys:          defaultKeyMap(),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = m.height - 7 // header + tabs + footer
		if m.detailOpen {
			m.updateViewport()
		}
		return m, nil

	case statusMsg:
		m.statusMsg = string(msg)
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Tab):
			if len(m.agents) > 0 {
				m.activeTab = (m.activeTab + 1) % len(m.agents)
				m.cursor = 0
				m.detailOpen = false
			}
			return m, nil

		case key.Matches(msg, m.keys.ShiftTab):
			if len(m.agents) > 0 {
				m.activeTab = (m.activeTab - 1 + len(m.agents)) % len(m.agents)
				m.cursor = 0
				m.detailOpen = false
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			findings := m.currentFindings()
			if m.cursor < len(findings)-1 {
				m.cursor++
				if m.detailOpen {
					m.updateViewport()
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.detailOpen {
					m.updateViewport()
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			m.detailOpen = !m.detailOpen
			if m.detailOpen {
				m.updateViewport()
			}
			return m, nil

		case key.Matches(msg, m.keys.Filter):
			m.cycleSeverityFilter()
			m.cursor = 0
			m.detailOpen = false
			return m, nil

		case key.Matches(msg, m.keys.Open):
			return m, m.openInGitHub()

		case key.Matches(msg, m.keys.Export):
			return m, m.exportMarkdown()

		case key.Matches(msg, m.keys.Copy):
			return m, m.copyToClipboard()
		}
	}

	if m.detailOpen {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	sections := []string{
		m.renderSummaryBar(),
		m.renderTabs(),
	}

	if m.detailOpen {
		sections = append(sections, m.viewport.View())
	} else {
		sections = append(sections, m.renderFindingList())
	}

	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ----- rendering helpers -----

var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	warningStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	infoStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	selectedStyle    = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	suggStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("237")).PaddingLeft(1)
)

func (m tuiModel) renderSummaryBar() string {
	pr := m.report.PR
	errors, warnings, infos := countSeverities(m.report.Findings)
	title := fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)
	counts := fmt.Sprintf("  %s  %s  %s",
		errorStyle.Render(fmt.Sprintf("✖ %d errors", errors)),
		warningStyle.Render(fmt.Sprintf("⚠ %d warnings", warnings)),
		infoStyle.Render(fmt.Sprintf("ℹ %d info", infos)),
	)
	return headerStyle.Render(title) + "\n" + counts + "\n"
}

func (m tuiModel) renderTabs() string {
	if len(m.agents) == 0 {
		return ""
	}
	var tabs []string
	for i, a := range m.agents {
		label := strings.ToUpper(a)
		count := len(m.agentFindings[a])
		text := fmt.Sprintf("%s (%d)", label, count)
		if i == m.activeTab {
			tabs = append(tabs, tabActiveStyle.Render(text))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(text))
		}
	}
	return strings.Join(tabs, "") + "\n"
}

func (m tuiModel) renderFindingList() string {
	findings := m.currentFindings()
	if len(findings) == 0 {
		filter := ""
		if m.severityFilter != "" {
			filter = fmt.Sprintf(" (filtered: %s)", m.severityFilter)
		}
		return dimStyle.Render(fmt.Sprintf("  No findings%s", filter)) + "\n"
	}

	maxLines := m.height - 8
	if maxLines < 1 {
		maxLines = 10
	}

	start := 0
	if m.cursor >= maxLines {
		start = m.cursor - maxLines + 1
	}

	var sb strings.Builder
	for i, f := range findings {
		if i < start || i >= start+maxLines {
			continue
		}
		line := m.renderFindingLine(f, i == m.cursor)
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func (m tuiModel) renderFindingLine(f agent.Finding, selected bool) string {
	icon, style := tuiSeverityStyle(f.Severity)
	loc := ""
	if l := formatLocation(f); l != "" {
		loc = dimStyle.Render(" "+l) + " "
	}
	rule := ""
	if f.RuleID != "" {
		rule = dimStyle.Render("["+f.RuleID+"]") + " "
	}
	msg := f.Message
	maxMsg := m.width - 30
	if maxMsg > 0 && len(msg) > maxMsg {
		msg = msg[:maxMsg] + "…"
	}
	line := fmt.Sprintf("  %s%s%s%s", style.Render(icon), loc, rule, msg)
	if selected {
		line = selectedStyle.Width(m.width).Render(line)
	}
	return line
}

func (m tuiModel) renderFooter() string {
	filterLabel := ""
	if m.severityFilter != "" {
		filterLabel = fmt.Sprintf(" | filter: %s", m.severityFilter)
	}
	status := m.statusMsg
	if status == "" {
		status = "tab next  j/k move  enter detail  o open  f filter  m export  c copy  q quit" + filterLabel
	}
	return "\n" + dimStyle.Render(status)
}

// ----- actions -----

func (m *tuiModel) currentFindings() []agent.Finding {
	if len(m.agents) == 0 {
		return nil
	}
	all := m.agentFindings[m.agents[m.activeTab]]
	if m.severityFilter == "" {
		return all
	}
	var out []agent.Finding
	for _, f := range all {
		if f.Severity == m.severityFilter {
			out = append(out, f)
		}
	}
	return out
}

func (m *tuiModel) updateViewport() {
	findings := m.currentFindings()
	if m.cursor >= len(findings) {
		return
	}
	f := findings[m.cursor]

	var sb strings.Builder
	icon, style := tuiSeverityStyle(f.Severity)
	fmt.Fprintf(&sb, "%s  %s\n", style.Render(icon+" "+string(f.Severity)), dimStyle.Render("["+f.RuleID+"]"))
	if loc := formatLocation(f); loc != "" {
		fmt.Fprintf(&sb, "%s\n", dimStyle.Render(loc))
	}
	fmt.Fprintf(&sb, "\n%s\n", f.Message)
	if f.Suggestion != "" {
		fmt.Fprintf(&sb, "\n%s\n", suggStyle.Render(f.Suggestion))
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoTop()
}

func (m *tuiModel) cycleSeverityFilter() {
	switch m.severityFilter {
	case "":
		m.severityFilter = agent.SeverityError
	case agent.SeverityError:
		m.severityFilter = agent.SeverityWarning
	case agent.SeverityWarning:
		m.severityFilter = agent.SeverityInfo
	default:
		m.severityFilter = ""
	}
}

func (m *tuiModel) openInGitHub() tea.Cmd {
	findings := m.currentFindings()
	if m.cursor >= len(findings) {
		return func() tea.Msg { return statusMsg("No finding selected") }
	}
	f := findings[m.cursor]
	if f.File == "" {
		return func() tea.Msg { return statusMsg("No file associated with this finding") }
	}
	pr := m.report.PR
	repo := fmt.Sprintf("%s/%s", pr.Owner, pr.Repo)
	target := f.File
	if f.LineStart > 0 {
		target = fmt.Sprintf("%s#L%d", f.File, f.LineStart)
	}
	return tea.ExecProcess(
		exec.Command("gh", "browse", target, "--repo", repo, "--commit", pr.HeadSHA),
		func(err error) tea.Msg {
			if err != nil {
				return statusMsg(fmt.Sprintf("Failed to open: %v", err))
			}
			return statusMsg("Opened in browser")
		},
	)
}

func (m *tuiModel) exportMarkdown() tea.Cmd {
	if m.markdownPath == "" {
		m.markdownPath = "./review-{pr_number}.md"
	}
	r := NewMarkdownRenderer(m.markdownPath)
	if err := r.Render(m.report); err != nil {
		msg := fmt.Sprintf("Export failed: %v", err)
		return func() tea.Msg { return statusMsg(msg) }
	}
	path := resolveOutputPath(m.markdownPath, m.report.PR.Number)
	msg := fmt.Sprintf("Exported to %s", path)
	return func() tea.Msg { return statusMsg(msg) }
}

func (m *tuiModel) copyToClipboard() tea.Cmd {
	findings := m.currentFindings()
	if m.cursor >= len(findings) {
		return func() tea.Msg { return statusMsg("No finding selected") }
	}
	f := findings[m.cursor]
	text := f.Message
	if f.Suggestion != "" {
		text += "\n\nSuggestion: " + f.Suggestion
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return func() tea.Msg { return statusMsg("No clipboard tool found (install xclip or xsel)") }
		}
	}

	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return func() tea.Msg { return statusMsg(fmt.Sprintf("Copy failed: %v", err)) }
	}
	return func() tea.Msg { return statusMsg("Copied!") }
}

func tuiSeverityStyle(sev agent.Severity) (string, lipgloss.Style) {
	switch sev {
	case agent.SeverityError:
		return "✖", errorStyle
	case agent.SeverityWarning:
		return "⚠", warningStyle
	default:
		return "ℹ", infoStyle
	}
}
