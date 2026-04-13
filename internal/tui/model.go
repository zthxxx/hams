package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles for the TUI.
var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	logPathStyle = lipgloss.NewStyle().Faint(true)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	skipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
)

// ApplyModel is the BubbleTea model for the hams apply progress view.
type ApplyModel struct {
	LogPath      string
	Providers    []string
	Current      int
	CurrentOp    string
	Completed    []StepResult
	Done         bool
	FinalSummary string
	Width        int
	Height       int
}

// StepResult records the outcome of one step.
type StepResult struct {
	Provider string
	Resource string
	Status   string // "ok", "fail", "skip"
	Message  string
}

// NewApplyModel creates a new model for the apply TUI.
func NewApplyModel(logPath string, providers []string) ApplyModel {
	return ApplyModel{
		LogPath:   logPath,
		Providers: providers,
	}
}

// Init implements tea.Model.
func (m ApplyModel) Init() tea.Cmd { //nolint:gocritic // BubbleTea requires value receiver for Model interface
	return nil
}

// Update implements tea.Model.
func (m ApplyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic // BubbleTea requires value receiver
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	case StepStartMsg:
		m.CurrentOp = msg.Operation
	case StepCompleteMsg:
		m.Completed = append(m.Completed, msg.Result)
		m.CurrentOp = ""
	case ProviderStartMsg:
		m.Current = msg.Index
	case ApplyDoneMsg:
		m.Done = true
		m.FinalSummary = msg.Summary
	}
	return m, nil
}

// View implements tea.Model.
func (m ApplyModel) View() string { //nolint:gocritic // BubbleTea requires value receiver
	var b strings.Builder

	// Sticky top: log file path.
	b.WriteString(logPathStyle.Render(fmt.Sprintf("📋 %s", m.LogPath)))
	b.WriteString("\n")

	// Header.
	b.WriteString(headerStyle.Render("hams apply"))
	b.WriteString("\n")

	// Progress bar.
	total := len(m.Providers)
	if total > 0 {
		progress := float64(m.Current) / float64(total)
		barWidth := 30
		filled := int(progress * float64(barWidth))
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		fmt.Fprintf(&b, "[%s] %d/%d providers\n", bar, m.Current, total)
	}

	// Current operation.
	if m.CurrentOp != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  → %s", m.CurrentOp)))
		b.WriteString("\n")
	}

	// Completed steps (last 10).
	showCount := 10
	start := 0
	if len(m.Completed) > showCount {
		start = len(m.Completed) - showCount
	}
	for _, r := range m.Completed[start:] {
		switch r.Status {
		case "ok":
			b.WriteString(okStyle.Render("  ✓ "))
		case "fail":
			b.WriteString(failStyle.Render("  ✗ "))
		case "skip":
			b.WriteString(skipStyle.Render("  - "))
		}
		fmt.Fprintf(&b, "%s %s\n", r.Resource, dimStyle.Render(r.Message))
	}

	// Final summary.
	if m.Done {
		b.WriteString("\n")
		b.WriteString(m.FinalSummary)
		b.WriteString("\n\nPress q to exit.")
	}

	return b.String()
}

// Messages for the BubbleTea model.

// StepStartMsg signals a new operation is starting.
type StepStartMsg struct {
	Operation string
}

// StepCompleteMsg signals an operation has finished.
type StepCompleteMsg struct {
	Result StepResult
}

// ProviderStartMsg signals a new provider is being processed.
type ProviderStartMsg struct {
	Index int
	Name  string
}

// ApplyDoneMsg signals the apply is complete.
type ApplyDoneMsg struct {
	Summary string
}
