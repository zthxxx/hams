// Package notify implements notification channels for alerting users during long-running operations.
package notify

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gen2brain/beeep"
)

// Channel is the interface for notification delivery.
type Channel interface {
	// Send delivers a notification with title and message.
	Send(title, message string) error
	// Name returns the channel name for logging.
	Name() string
}

// Manager coordinates sending notifications across all configured channels.
type Manager struct {
	channels []Channel
}

// NewManager creates a notification manager with the default channels.
func NewManager(barkToken string) *Manager {
	m := &Manager{}

	// Desktop notification is mandatory (uses beeep for cross-platform support).
	m.channels = append(m.channels, &desktopNotifier{})

	// Bark is optional.
	if barkToken != "" {
		m.channels = append(m.channels, &barkChannel{token: barkToken})
	}

	return m
}

// Notify sends a notification to all configured channels.
func (m *Manager) Notify(title, message string) {
	for _, ch := range m.channels {
		if err := ch.Send(title, message); err != nil {
			slog.Warn("notification failed", "channel", ch.Name(), "error", err)
		}
	}
}

// NotifyApplyComplete sends a summary notification after apply finishes.
func (m *Manager) NotifyApplyComplete(installed, failed, skipped int) {
	status := "success"
	if failed > 0 {
		status = "partial failure"
	}
	m.Notify("hams apply "+status,
		fmt.Sprintf("%d installed, %d failed, %d skipped", installed, failed, skipped))
}

// NotifyInteractionRequired alerts the user that input is needed.
func (m *Manager) NotifyInteractionRequired(providerName, operation string) {
	m.Notify("hams: input required",
		fmt.Sprintf("Provider %s needs attention: %s", providerName, operation))
}

// desktopNotifier uses gen2brain/beeep for cross-platform desktop notifications.
// Supports macOS (terminal-notifier/osascript), Linux (notify-send/dbus), Windows (toast).
type desktopNotifier struct{}

func (d *desktopNotifier) Name() string { return "desktop" }

func (d *desktopNotifier) Send(title, message string) error {
	return beeep.Notify(title, message, "")
}

// barkChannel sends push notifications via Bark app (iOS).
type barkChannel struct {
	token string
}

func (b *barkChannel) Name() string { return "bark" }

func (b *barkChannel) Send(title, message string) error {
	url := fmt.Sprintf("https://api.day.app/%s/%s/%s",
		b.token,
		strings.ReplaceAll(title, " ", "%20"),
		strings.ReplaceAll(message, " ", "%20"),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url) //nolint:noctx // Bark API uses simple GET, context not needed
	if err != nil {
		return fmt.Errorf("bark notification: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body not needed

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bark notification: HTTP %d", resp.StatusCode)
	}
	return nil
}
