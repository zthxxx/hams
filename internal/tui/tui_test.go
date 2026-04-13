package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewApplyModel(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", []string{"brew", "apt"})
	if m.LogPath != "/tmp/log" {
		t.Errorf("LogPath = %q, want /tmp/log", m.LogPath)
	}
	if len(m.Providers) != 2 {
		t.Errorf("Providers len = %d, want 2", len(m.Providers))
	}
	if m.Done {
		t.Error("Done should be false initially")
	}
}

func TestApplyModel_Init(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", nil)
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init should return nil cmd")
	}
}

func TestApplyModel_Update_WindowSize(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model := updated.(ApplyModel) //nolint:errcheck // test assertion
	if model.Width != 80 || model.Height != 24 {
		t.Errorf("size = %dx%d, want 80x24", model.Width, model.Height)
	}
}

func TestApplyModel_Update_StepMessages(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", []string{"brew"})

	// StepStart.
	updated, _ := m.Update(StepStartMsg{Operation: "installing htop"})
	model := updated.(ApplyModel) //nolint:errcheck // test assertion
	if model.CurrentOp != "installing htop" {
		t.Errorf("CurrentOp = %q, want 'installing htop'", model.CurrentOp)
	}

	// StepComplete.
	updated, _ = model.Update(StepCompleteMsg{Result: StepResult{
		Provider: "brew", Resource: "htop", Status: statusOK, Message: "installed",
	}})
	model = updated.(ApplyModel) //nolint:errcheck // test assertion
	if len(model.Completed) != 1 {
		t.Fatalf("Completed len = %d, want 1", len(model.Completed))
	}
	if model.Completed[0].Resource != "htop" {
		t.Errorf("Completed[0].Resource = %q", model.Completed[0].Resource)
	}
	if model.CurrentOp != "" {
		t.Error("CurrentOp should be cleared after step complete")
	}
}

func TestApplyModel_Update_ProviderStart(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", []string{"brew", "apt"})
	updated, _ := m.Update(ProviderStartMsg{Index: 1, Name: "apt"})
	model := updated.(ApplyModel) //nolint:errcheck // test assertion
	if model.Current != 1 {
		t.Errorf("Current = %d, want 1", model.Current)
	}
}

func TestApplyModel_Update_ApplyDone(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", nil)
	updated, _ := m.Update(ApplyDoneMsg{Summary: "All done!"})
	model := updated.(ApplyModel) //nolint:errcheck // test assertion
	if !model.Done {
		t.Error("Done should be true")
	}
	if model.FinalSummary != "All done!" {
		t.Errorf("FinalSummary = %q", model.FinalSummary)
	}
}

func TestApplyModel_View_ContainsLogPath(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/test.log", []string{"brew"})
	view := m.View()
	if !strings.Contains(view, "/tmp/test.log") {
		t.Error("View should contain log path")
	}
}

func TestApplyModel_View_ContainsProgress(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", []string{"brew", "apt", "npm"})
	m.Current = 1
	view := m.View()
	if !strings.Contains(view, "1/3") {
		t.Error("View should contain progress '1/3'")
	}
}

func TestApplyModel_View_ShowsLast10Steps(t *testing.T) {
	t.Parallel()
	m := NewApplyModel("/tmp/log", []string{"brew"})
	for i := range 15 {
		m.Completed = append(m.Completed, StepResult{
			Provider: "brew", Resource: "pkg" + string(rune('a'+i)), Status: statusOK,
		})
	}
	view := m.View()
	// Should show last 10 only; first 5 should be truncated.
	if strings.Contains(view, "pkga") {
		t.Error("View should not show pkga (truncated)")
	}
}

// --- PickerModel tests ---

func TestNewPickerModel_LLMPreSelected(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"devtools", "cli"}, []string{"network"})
	if len(m.Tags) != 3 {
		t.Fatalf("Tags len = %d, want 3", len(m.Tags))
	}
	// LLM tags should be pre-selected.
	if !m.Tags[0].Selected || !m.Tags[0].IsLLM {
		t.Error("LLM tag 'devtools' should be selected and marked IsLLM")
	}
	// Existing tags should not be pre-selected.
	if m.Tags[2].Selected || m.Tags[2].IsLLM {
		t.Error("existing tag 'network' should not be selected")
	}
}

func TestNewPickerModel_Deduplication(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"cli", "cli"}, []string{"cli", "other"})
	if len(m.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2 (deduplicated)", len(m.Tags))
	}
}

func TestPickerModel_SelectedTags(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"a", "b"}, []string{"c"})
	selected := m.SelectedTags()
	if len(selected) != 2 {
		t.Errorf("SelectedTags len = %d, want 2 (LLM tags)", len(selected))
	}
}

func TestPickerModel_ToggleSelection(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"a"}, nil)
	// Toggle off.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.Tags[0].Selected {
		t.Error("tag should be deselected after space")
	}
}

func TestPickerModel_Navigation(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"a", "b", "c"}, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(PickerModel) //nolint:errcheck // test assertion
	if model.Cursor != 1 {
		t.Errorf("Cursor = %d, want 1 after down", model.Cursor)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(PickerModel) //nolint:errcheck // test assertion
	if model.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0 after up", model.Cursor)
	}
}

func TestPickerModel_TabToInput(t *testing.T) {
	t.Parallel()
	m := NewPickerModel([]string{"a"}, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := updated.(PickerModel) //nolint:errcheck // test assertion
	if !model.InputActive {
		t.Error("InputActive should be true after tab")
	}
}

func TestPickerModel_FreeTextInput(t *testing.T) {
	t.Parallel()
	m := PickerModel{InputActive: true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := updated.(PickerModel) //nolint:errcheck // test assertion
	if model.Input != "x" {
		t.Errorf("Input = %q, want 'x'", model.Input)
	}
	// Enter to add tag.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(PickerModel) //nolint:errcheck // test assertion
	if len(model.Tags) != 1 || model.Tags[0].Name != "x" {
		t.Errorf("Tags = %v, want [{x}]", model.Tags)
	}
	if model.InputActive {
		t.Error("InputActive should be false after adding tag")
	}
}

// --- LogViewModel tests ---

func TestLogViewModel_AddSection(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	idx := m.AddSection("test")
	if idx != 0 {
		t.Errorf("idx = %d, want 0", idx)
	}
	if len(m.Sections) != 1 {
		t.Fatal("expected 1 section")
	}
	if m.Sections[0].Title != "test" {
		t.Errorf("Title = %q", m.Sections[0].Title)
	}
	if m.Sections[0].Status != "running" {
		t.Errorf("Status = %q, want 'running'", m.Sections[0].Status)
	}
}

func TestLogViewModel_AppendLine(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	m.AddSection("test")
	m.AppendLine(0, "line1")
	m.AppendLine(0, "line2")
	if len(m.Sections[0].Lines) != 2 {
		t.Errorf("Lines = %d, want 2", len(m.Sections[0].Lines))
	}
	// Out of bounds should not panic.
	m.AppendLine(99, "nope")
}

func TestLogViewModel_SetStatus(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	m.AddSection("test")
	m.SetStatus(0, statusOK)
	if m.Sections[0].Status != statusOK {
		t.Errorf("Status = %q, want %q", m.Sections[0].Status, statusOK)
	}
	// Out of bounds should not panic.
	m.SetStatus(99, statusFail)
}

func TestLogViewModel_ToggleExpand(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	m.AddSection("test")
	m.AppendLine(0, "line")

	// Enter to toggle expand.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(LogViewModel) //nolint:errcheck // test assertion
	if !model.Sections[0].Expanded {
		t.Error("section should be expanded after enter")
	}
}

func TestLogViewModel_View_ShowsLineCount(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	m.AddSection("build")
	m.AppendLine(0, "line1")
	m.AppendLine(0, "line2")
	view := m.View()
	if !strings.Contains(view, "2 lines") {
		t.Error("View should show line count")
	}
}

func TestLogViewModel_MaxLines_Truncation(t *testing.T) {
	t.Parallel()
	m := NewLogViewModel()
	m.AddSection("test")
	m.Sections[0].MaxLines = 5
	m.Sections[0].Expanded = true
	for range 20 {
		m.AppendLine(0, "line")
	}
	view := m.View()
	if !strings.Contains(view, "hidden") {
		t.Error("View should mention hidden lines when truncated")
	}
}
