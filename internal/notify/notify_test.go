package notify

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// TestBarkSend_HappyPath asserts barkChannel.Send hits the right
// URL path (`/<token>/<title>/<message>`) against a test server and
// returns nil on 200.
//
// NOT Parallel because we swap the package-global barkBaseURL.
// Sharing that across goroutines would race; the tests are cheap
// enough that sequential execution is fine.
func TestBarkSend_HappyPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Cleanup(withBarkBaseURL(srv.URL))

	b := &barkChannel{token: "tok"}
	if err := b.Send("hello", "world"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if want := "/tok/hello/world"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

// TestBarkSend_URLEscaping asserts special characters in title/
// message are url.PathEscape'd rather than silently passed through.
// The pre-cycle-107 implementation only replaced spaces, so `#` (URL
// fragment separator), `?` (query delimiter), and `/` (segment
// separator) would truncate or split the message. This is the
// primary regression-gate test for cycle 107. `&` is left unescaped
// by url.PathEscape per RFC 3986 sub-delims — that's intentional
// and correct for URL path segments.
func TestBarkSend_URLEscaping(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path is the decoded path; r.URL.RawPath preserves escapes.
		if r.URL.RawPath != "" {
			gotPath = r.URL.RawPath
		} else {
			gotPath = r.URL.Path
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Cleanup(withBarkBaseURL(srv.URL))

	b := &barkChannel{token: "tok"}
	if err := b.Send("hams apply #1 failed", "bad/chars?yes"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The escape MUST reach the server with encoded hazards — `#`,
	// `?`, `/` MUST NOT appear as raw path delimiters between the
	// token/title/message segments.
	for _, hazard := range []string{"#", "?"} {
		if strings.Contains(gotPath, hazard) {
			t.Errorf("raw path %q still contains unescaped hazard %q — url.PathEscape failed",
				gotPath, hazard)
		}
	}

	// Title escaping: `#` → `%23`; `/` in message → `%2F` (or %2f).
	if !strings.Contains(strings.ToLower(gotPath), "%23") {
		t.Errorf("expected %%23 (escaped `#`) in path %q", gotPath)
	}
	if !strings.Contains(strings.ToLower(gotPath), "%2f") {
		t.Errorf("expected %%2f (escaped `/`) in path %q", gotPath)
	}
}

// TestBarkSend_Non200ReturnsError asserts barkChannel.Send returns
// a typed error when the upstream responds with a non-2xx status.
// Manager.Notify logs errors but continues, so tests that only went
// through the Manager couldn't assert this branch.
func TestBarkSend_Non200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	t.Cleanup(withBarkBaseURL(srv.URL))

	b := &barkChannel{token: "tok"}
	err := b.Send("t", "m")
	if err == nil {
		t.Fatalf("Send with 502 response should return error, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error %v should mention status 502", err)
	}
}

// TestBarkSend_NetworkErrorReturnsError asserts Send surfaces the
// underlying network error when the remote is unreachable.
func TestBarkSend_NetworkErrorReturnsError(t *testing.T) {
	t.Cleanup(withBarkBaseURL("http://127.0.0.1:1")) // unreachable port

	b := &barkChannel{token: "tok"}
	if err := b.Send("t", "m"); err == nil {
		t.Fatalf("Send against unreachable host should return error, got nil")
	}
}

// withBarkBaseURL swaps barkBaseURL for the duration of a test and
// returns a cleanup func that restores the original.
func withBarkBaseURL(newURL string) func() {
	original := barkBaseURL
	barkBaseURL = newURL
	return func() { barkBaseURL = original }
}
