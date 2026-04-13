package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Key constants shared across TUI models.
const (
	keyCtrlC = "ctrl+c"
	keyEnter = "enter"
	keyQuit  = "q"
)

// Status constants shared across TUI models.
const (
	statusOK   = "ok"
	statusFail = "fail"
	statusSkip = "skip"
)

// LogSection represents a collapsible log section in the TUI.
type LogSection struct {
	Title    string
	Lines    []string
	Expanded bool
	Status   string // "running", "ok", "fail"
	MaxLines int    // Max lines to show when expanded (0 = all).
}

// LogViewModel is a BubbleTea model for collapsible log output.
type LogViewModel struct {
	Sections []LogSection
	Cursor   int
}

// NewLogViewModel creates a new log view model.
func NewLogViewModel() LogViewModel {
	return LogViewModel{}
}

// AddSection adds a new collapsible section.
func (m *LogViewModel) AddSection(title string) int {
	idx := len(m.Sections)
	m.Sections = append(m.Sections, LogSection{
		Title:    title,
		Status:   "running",
		MaxLines: 20,
	})
	return idx
}

// AppendLine adds a log line to a section.
func (m *LogViewModel) AppendLine(sectionIdx int, line string) {
	if sectionIdx >= 0 && sectionIdx < len(m.Sections) {
		m.Sections[sectionIdx].Lines = append(m.Sections[sectionIdx].Lines, line)
	}
}

// SetStatus updates a section's status.
func (m *LogViewModel) SetStatus(sectionIdx int, status string) {
	if sectionIdx >= 0 && sectionIdx < len(m.Sections) {
		m.Sections[sectionIdx].Status = status
	}
}

// Init implements tea.Model.
func (m LogViewModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m LogViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case keyCtrlC, keyQuit:
			return m, tea.Quit
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if m.Cursor < len(m.Sections)-1 {
				m.Cursor++
			}
		case keyEnter, " ":
			if m.Cursor < len(m.Sections) {
				m.Sections[m.Cursor].Expanded = !m.Sections[m.Cursor].Expanded
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m LogViewModel) View() string {
	var b strings.Builder

	for i, s := range m.Sections {
		cursor := "  "
		if i == m.Cursor {
			cursor = cursorStyle.Render("▸ ")
		}

		icon := "▶"
		if s.Expanded {
			icon = "▼"
		}

		statusIcon := dimStyle.Render("⋯")
		switch s.Status {
		case statusOK:
			statusIcon = okStyle.Render("✓")
		case statusFail:
			statusIcon = failStyle.Render("✗")
		}

		fmt.Fprintf(&b, "%s%s %s %s (%d lines)\n", cursor, statusIcon, icon, s.Title, len(s.Lines))

		if s.Expanded {
			lines := s.Lines
			if s.MaxLines > 0 && len(lines) > s.MaxLines {
				lines = lines[len(lines)-s.MaxLines:]
				fmt.Fprintf(&b, "    %s\n", dimStyle.Render(fmt.Sprintf("... %d lines hidden", len(s.Lines)-s.MaxLines)))
			}
			for _, line := range lines {
				fmt.Fprintf(&b, "    %s\n", dimStyle.Render(line))
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓ navigate  space/enter toggle  q quit"))
	b.WriteString("\n")

	return b.String()
}
