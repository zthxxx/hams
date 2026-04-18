package vscodeext

import (
	"context"

	"github.com/zthxxx/hams/internal/provider"
)

// CodeHandler exposes the VS Code extensions provider behind the
// `hams code` CLI entry point. Per the onboarding-auto-init follow-up
// (CLAUDE.md task list), users should type `hams code install <ext>`
// instead of the legacy `hams code-ext install <ext>`. The wrapping is
// CLI-surface only — the underlying Provider still carries
// Manifest.Name="code-ext" and FilePrefix="vscodeext" so existing
// hamsfiles + state files keep working unchanged.
//
// Cursor support, when added, SHALL ship as a separate `cursor`
// provider rather than a `cli_command: cursor` overload of this
// handler — that is the explicit guidance from CLAUDE.md.
type CodeHandler struct {
	underlying *Provider
}

// NewCodeHandler wraps an existing vscodeext.Provider so the same
// instance handles both apply/refresh (via Provider) and CLI dispatch
// (via the wrapper's ProviderHandler implementation).
func NewCodeHandler(p *Provider) *CodeHandler {
	return &CodeHandler{underlying: p}
}

// Name is the CLI verb users type — `hams code`.
func (c *CodeHandler) Name() string { return "code" }

// DisplayName returns "VS Code" — distinct from the underlying
// provider's "VS Code Extensions" because the user-facing entry point
// is the editor itself, not the extension subsystem.
func (c *CodeHandler) DisplayName() string { return "VS Code" }

// HandleCommand passes args through to the underlying VS Code provider.
// The wrapper exists purely to project the new `hams code` name onto
// the existing implementation; argument parsing stays identical.
func (c *CodeHandler) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	return c.underlying.HandleCommand(ctx, args, hamsFlags, flags)
}
