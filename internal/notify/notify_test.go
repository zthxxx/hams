package notify

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mockChannel records calls for testing.
type mockChannel struct {
	name     string
	calls    []mockCall
	failWith error
}

type mockCall struct {
	title, message string
}

func (m *mockChannel) Name() string { return m.name }
func (m *mockChannel) Send(title, message string) error {
	m.calls = append(m.calls, mockCall{title, message})
	return m.failWith
}

func TestManager_Notify_SingleChannel(t *testing.T) {
	t.Parallel()
	ch := &mockChannel{name: "test"}
	m := &Manager{channels: []Channel{ch}}

	m.Notify("title", "body")

	if len(ch.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(ch.calls))
	}
	if ch.calls[0].title != "title" || ch.calls[0].message != "body" {
		t.Errorf("call = %+v", ch.calls[0])
	}
}

func TestManager_Notify_MultipleChannels(t *testing.T) {
	t.Parallel()
	ch1 := &mockChannel{name: "ch1"}
	ch2 := &mockChannel{name: "ch2"}
	m := &Manager{channels: []Channel{ch1, ch2}}

	m.Notify("t", "m")

	if len(ch1.calls) != 1 || len(ch2.calls) != 1 {
		t.Errorf("expected 1 call each, got ch1=%d, ch2=%d", len(ch1.calls), len(ch2.calls))
	}
}

func TestManager_Notify_ChannelErrorDoesNotBlock(t *testing.T) {
	t.Parallel()
	ch1 := &mockChannel{name: "failing", failWith: errors.New("fail")}
	ch2 := &mockChannel{name: "working"}
	m := &Manager{channels: []Channel{ch1, ch2}}

	m.Notify("t", "m")

	// ch2 should still receive the notification even though ch1 failed.
	if len(ch2.calls) != 1 {
		t.Errorf("ch2 should have received 1 call, got %d", len(ch2.calls))
	}
}

func TestManager_NotifyApplyComplete_Success(t *testing.T) {
	t.Parallel()
	ch := &mockChannel{name: "test"}
	m := &Manager{channels: []Channel{ch}}

	m.NotifyApplyComplete(5, 0, 2)

	if len(ch.calls) != 1 {
		t.Fatal("expected 1 call")
	}
	if !strings.Contains(ch.calls[0].title, "success") {
		t.Errorf("title should contain 'success', got %q", ch.calls[0].title)
	}
	if !strings.Contains(ch.calls[0].message, "5 installed") {
		t.Errorf("message should contain '5 installed', got %q", ch.calls[0].message)
	}
}

func TestManager_NotifyApplyComplete_PartialFailure(t *testing.T) {
	t.Parallel()
	ch := &mockChannel{name: "test"}
	m := &Manager{channels: []Channel{ch}}

	m.NotifyApplyComplete(3, 2, 1)

	if !strings.Contains(ch.calls[0].title, "partial failure") {
		t.Errorf("title should contain 'partial failure', got %q", ch.calls[0].title)
	}
}

func TestManager_NotifyInteractionRequired(t *testing.T) {
	t.Parallel()
	ch := &mockChannel{name: "test"}
	m := &Manager{channels: []Channel{ch}}

	m.NotifyInteractionRequired("homebrew", "sign-in")

	if len(ch.calls) != 1 {
		t.Fatal("expected 1 call")
	}
	if !strings.Contains(ch.calls[0].title, "input required") {
		t.Errorf("title should contain 'input required', got %q", ch.calls[0].title)
	}
	if !strings.Contains(ch.calls[0].message, "homebrew") {
		t.Errorf("message should contain provider name, got %q", ch.calls[0].message)
	}
}

func TestNewManager_DesktopOnly(t *testing.T) {
	t.Parallel()
	m := NewManager("")
	if len(m.channels) != 1 {
		t.Errorf("expected 1 channel (desktop), got %d", len(m.channels))
	}
}

func TestNewManager_WithBark(t *testing.T) {
	t.Parallel()
	m := NewManager("test-token")
	if len(m.channels) != 2 {
		t.Errorf("expected 2 channels (desktop + bark), got %d", len(m.channels))
	}
}

func TestNotifyApplyComplete_MessageFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		installed, failed, skipped int
		wantStatus                 string
	}{
		{10, 0, 0, "success"},
		{5, 3, 2, "partial failure"},
		{0, 1, 0, "partial failure"},
		{0, 0, 5, "success"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_%d_%d", tt.installed, tt.failed, tt.skipped), func(t *testing.T) {
			t.Parallel()
			ch := &mockChannel{name: "test"}
			m := &Manager{channels: []Channel{ch}}
			m.NotifyApplyComplete(tt.installed, tt.failed, tt.skipped)
			if !strings.Contains(ch.calls[0].title, tt.wantStatus) {
				t.Errorf("title %q should contain %q", ch.calls[0].title, tt.wantStatus)
			}
		})
	}
}

// TestDesktopNotifier_Name asserts the Channel.Name() contract for
// the desktop notifier (needed by Manager.Notify for logging).
// Send() is NOT covered here — it would trigger an actual OS-level
// notification and requires a graphical session on Linux.
func TestDesktopNotifier_Name(t *testing.T) {
	t.Parallel()
	d := &desktopNotifier{}
	if got := d.Name(); got != "desktop" {
		t.Errorf("Name() = %q, want 'desktop'", got)
	}
}

// TestBarkChannel_Name asserts the Channel.Name() contract for the
// Bark notifier. The name appears in slog.Info lines when each
// Manager.Notify iteration fires.
func TestBarkChannel_Name(t *testing.T) {
	t.Parallel()
	b := &barkChannel{token: "any"}
	if got := b.Name(); got != "bark" {
		t.Errorf("Name() = %q, want 'bark'", got)
	}
}

// Note on Bark Send() coverage: barkChannel.Send uses a hardcoded
// api.day.app URL without an injectable http.Client. Covering the
// non-200 branch would require DI refactoring of the HTTP seam —
// out of scope here because Bark is a v1.1-deferred feature (see
// openspec/specs/tui-logging/spec.md). Adding DI now would lock in
// an API shape before v1.1 wires the channel into apply/refresh.
