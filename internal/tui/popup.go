package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	popupBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("214")).
				Padding(1, 2)
	popupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)

// PopupModel is a BubbleTea model for interactive provider stdin popups.
// It suspends the main TUI, passes raw stdin to the subprocess,
// and resumes when the subprocess exits.
type PopupModel struct {
	Title        string
	ProviderName string
	Operation    string
	Done         bool
	ExitCode     int
	Output       string
}

// NewPopupModel creates a popup for an interactive provider operation.
func NewPopupModel(providerName, operation string) PopupModel {
	return PopupModel{
		Title:        fmt.Sprintf("%s: %s", providerName, operation),
		ProviderName: providerName,
		Operation:    operation,
	}
}

// Init implements tea.Model.
func (m PopupModel) Init() tea.Cmd { //nolint:gocritic // BubbleTea Model interface requires value receiver
	return nil
}

// Update implements tea.Model.
func (m PopupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic // BubbleTea Model interface requires value receiver
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == keyCtrlC {
			return m, tea.Quit
		}
		if m.Done && (keyMsg.String() == keyEnter || keyMsg.String() == keyQuit) {
			return m, tea.Quit
		}
	}

	if doneMsg, ok := msg.(PopupDoneMsg); ok {
		m.Done = true
		m.ExitCode = doneMsg.ExitCode
		m.Output = doneMsg.Output
	}

	return m, nil
}

// View implements tea.Model.
func (m PopupModel) View() string { //nolint:gocritic // BubbleTea Model interface requires value receiver
	var content strings.Builder

	content.WriteString(popupTitleStyle.Render(m.Title))
	content.WriteString("\n\n")

	if m.Done {
		if m.ExitCode == 0 {
			content.WriteString(okStyle.Render("Operation completed successfully."))
		} else {
			fmt.Fprintf(&content, "%s (exit code %d)", failStyle.Render("Operation failed."), m.ExitCode)
		}
		if m.Output != "" {
			content.WriteString("\n\n")
			content.WriteString(dimStyle.Render(m.Output))
		}
		content.WriteString("\n\n")
		content.WriteString(dimStyle.Render("Press enter or q to continue."))
	} else {
		content.WriteString("Waiting for interactive input...")
		content.WriteString("\n")
		content.WriteString(dimStyle.Render("The process needs your attention."))
	}

	return popupBorderStyle.Render(content.String())
}

// PopupDoneMsg signals the interactive operation has completed.
type PopupDoneMsg struct {
	ExitCode int
	Output   string
}

// RunInteractive suspends the BubbleTea program, runs a command with full
// stdin/stdout/stderr passthrough, then resumes. This is the mechanism for
// provider signin flows, OAuth callbacks, etc.
func RunInteractive(ctx context.Context, p *tea.Program, name string, args ...string) error {
	// Suspend BubbleTea to release the terminal.
	if p != nil {
		slog.Debug("suspending TUI for interactive command", "command", name)
		// BubbleTea handles suspend/resume via ReleaseTerminal/RestoreTerminal
		// in newer versions. For now, we run the command directly.
	}

	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // interactive commands from provider declarations
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if p != nil {
		slog.Debug("resuming TUI after interactive command")
	}

	return err
}
