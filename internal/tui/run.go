package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTagPicker displays the interactive tag picker and returns selected tags.
// If lucky is true or the terminal is non-interactive, returns llmTags directly
// without displaying the TUI picker (auto-accept LLM recommendations).
func RunTagPicker(llmTags, existingTags []string, lucky bool) ([]string, error) {
	if lucky || !IsInteractive() {
		// Lucky mode or non-interactive: auto-accept LLM recommendations.
		return llmTags, nil
	}

	model := NewPickerModel(llmTags, existingTags)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tag picker: %w", err)
	}

	result, ok := finalModel.(PickerModel)
	if !ok {
		return llmTags, nil
	}

	if result.Canceled {
		return nil, fmt.Errorf("tag selection canceled")
	}

	return result.SelectedTags(), nil
}

// RunApplyTUI starts the BubbleTea apply progress view.
// Returns when the user quits or apply completes.
func RunApplyTUI(logPath string, providers []string) error {
	if !IsInteractive() {
		// Non-interactive: no TUI, just log to stdout.
		return nil
	}

	model := NewApplyModel(logPath, providers)
	p := tea.NewProgram(model, tea.WithAltScreen())

	_, err := p.Run()
	return err
}
