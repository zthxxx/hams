package provider

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/zthxxx/hams/internal/state"
)

// Hook represents a pre/post-install or pre/post-update hook on a resource.
type Hook struct {
	Type    HookType
	Command string
	Defer   bool // If true, execute after all resources in the provider are done.
}

// HookType categorizes when a hook fires.
type HookType int

const (
	// HookPreInstall fires before a resource is installed.
	HookPreInstall HookType = iota
	// HookPostInstall fires after a resource is installed.
	HookPostInstall
	// HookPreUpdate fires before a resource is updated/upgraded.
	HookPreUpdate
	// HookPostUpdate fires after a resource is updated/upgraded.
	HookPostUpdate
)

// HookSet holds all hooks for a resource, grouped by type.
type HookSet struct {
	PreInstall  []Hook
	PostInstall []Hook
	PreUpdate   []Hook
	PostUpdate  []Hook
}

// RunPreInstallHooks executes pre-install hooks for a resource.
// Returns an error if any hook fails (blocks the install).
func RunPreInstallHooks(ctx context.Context, hooks []Hook, resourceID string) error {
	for _, h := range hooks {
		if h.Defer {
			continue // Deferred hooks are collected, not run here.
		}
		if err := runHook(ctx, h, resourceID); err != nil {
			return fmt.Errorf("pre-install hook for %s failed: %w", resourceID, err)
		}
	}
	return nil
}

// RunPostInstallHooks executes non-deferred post-install hooks.
// Returns hook-failed errors without stopping.
func RunPostInstallHooks(ctx context.Context, hooks []Hook, resourceID string, sf *state.File) error {
	for _, h := range hooks {
		if h.Defer {
			continue
		}
		if err := runHook(ctx, h, resourceID); err != nil {
			slog.Error("post-install hook failed", "resource", resourceID, "error", err)
			sf.SetResource(resourceID, state.StateHookFailed, state.WithError(err.Error()))
			return fmt.Errorf("post-install hook for %s failed: %w", resourceID, err)
		}
	}
	return nil
}

// CollectDeferredHooks returns all hooks with defer=true from the given hook set.
func CollectDeferredHooks(hooks []Hook) []DeferredHook {
	var deferred []DeferredHook
	for _, h := range hooks {
		if h.Defer {
			deferred = append(deferred, DeferredHook{Hook: h})
		}
	}
	return deferred
}

// DeferredHook wraps a hook with its resource context for deferred execution.
type DeferredHook struct {
	Hook       Hook
	ResourceID string
}

// RunDeferredHooks executes all deferred hooks in order after the provider finishes.
func RunDeferredHooks(ctx context.Context, deferred []DeferredHook, sf *state.File) []error {
	var errs []error
	for _, dh := range deferred {
		if err := runHook(ctx, dh.Hook, dh.ResourceID); err != nil {
			slog.Error("deferred hook failed", "resource", dh.ResourceID, "error", err)
			sf.SetResource(dh.ResourceID, state.StateHookFailed, state.WithError(err.Error()))
			errs = append(errs, fmt.Errorf("deferred hook for %s failed: %w", dh.ResourceID, err))
		}
	}
	return errs
}

func runHook(ctx context.Context, h Hook, resourceID string) error {
	slog.Debug("running hook", "type", h.Type, "resource", resourceID, "command", h.Command)
	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command) //nolint:gosec // hook commands are user-defined in hamsfile, not external input
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %q: %w\noutput: %s", h.Command, err, string(output))
	}
	return nil
}
