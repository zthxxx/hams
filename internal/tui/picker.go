package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tag picker styles.
var (
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	unselectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	cursorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	inputStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// PickerModel is a BubbleTea model for multi-select tag picking.
type PickerModel struct {
	Tags        []PickerTag
	Cursor      int
	Input       string // Free-text input for new tags.
	InputActive bool   // Whether the free-text input is focused.
	Done        bool
	Canceled    bool
}

// PickerTag represents a tag option in the picker.
type PickerTag struct {
	Name     string
	Selected bool
	IsLLM    bool // True if LLM-recommended (pre-selected).
}

// NewPickerModel creates a picker with LLM-recommended and existing tags.
func NewPickerModel(llmTags, existingTags []string) PickerModel {
	seen := make(map[string]bool)
	var tags []PickerTag

	for _, t := range llmTags {
		if !seen[t] {
			tags = append(tags, PickerTag{Name: t, Selected: true, IsLLM: true})
			seen[t] = true
		}
	}

	for _, t := range existingTags {
		if !seen[t] {
			tags = append(tags, PickerTag{Name: t, Selected: false})
			seen[t] = true
		}
	}

	return PickerModel{Tags: tags}
}

// SelectedTags returns the names of all selected tags.
func (m PickerModel) SelectedTags() []string {
	var selected []string
	for _, t := range m.Tags {
		if t.Selected {
			selected = append(selected, t.Name)
		}
	}
	return selected
}

// Init implements tea.Model.
func (m PickerModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "ctrl+c":
		m.Canceled = true
		return m, tea.Quit
	case "enter":
		if m.InputActive && m.Input != "" {
			m.Tags = append(m.Tags, PickerTag{Name: m.Input, Selected: true})
			m.Input = ""
			m.InputActive = false
		} else {
			m.Done = true
			return m, tea.Quit
		}
	case "up", "k":
		if !m.InputActive && m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if !m.InputActive && m.Cursor < len(m.Tags)-1 {
			m.Cursor++
		}
	case " ":
		if !m.InputActive && m.Cursor < len(m.Tags) {
			m.Tags[m.Cursor].Selected = !m.Tags[m.Cursor].Selected
		}
	case "tab":
		m.InputActive = !m.InputActive
	case "backspace":
		if m.InputActive && m.Input != "" {
			m.Input = m.Input[:len(m.Input)-1]
		}
	default:
		if m.InputActive && len(keyMsg.String()) == 1 {
			m.Input += keyMsg.String()
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m PickerModel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Select tags (space=toggle, tab=new tag, enter=confirm)"))
	b.WriteString("\n\n")

	for i, t := range m.Tags {
		cursor := "  "
		if i == m.Cursor && !m.InputActive {
			cursor = cursorStyle.Render("▸ ")
		}

		checkbox := "[ ]"
		style := unselectedStyle
		if t.Selected {
			checkbox = "[✓]"
			style = selectedStyle
		}

		label := style.Render(t.Name)
		suffix := ""
		if t.IsLLM {
			suffix = dimStyle.Render(" (recommended)")
		}

		fmt.Fprintf(&b, "%s%s %s%s\n", cursor, checkbox, label, suffix)
	}

	b.WriteString("\n")
	if m.InputActive {
		fmt.Fprintf(&b, "%s New tag: %s▏\n", cursorStyle.Render("▸"), inputStyle.Render(m.Input))
	} else {
		b.WriteString("  [tab] Add new tag\n")
	}

	return b.String()
}
