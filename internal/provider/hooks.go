package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

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

// String returns the human-readable name of the hook type.
func (h HookType) String() string {
	switch h {
	case HookPreInstall:
		return "pre-install"
	case HookPostInstall:
		return "post-install"
	case HookPreUpdate:
		return "pre-update"
	case HookPostUpdate:
		return "post-update"
	default:
		return fmt.Sprintf("HookType(%d)", int(h))
	}
}

// HookSet holds all hooks for a resource, grouped by type.
type HookSet struct {
	PreInstall  []Hook
	PostInstall []Hook
	PreUpdate   []Hook
	PostUpdate  []Hook
}

// RunPreInstallHooks executes pre-install hooks for a resource.
func RunPreInstallHooks(ctx context.Context, hooks []Hook, resourceID string) error {
	return runPreHooks(ctx, hooks, resourceID, "pre-install")
}

// RunPreUpdateHooks executes pre-update hooks for a resource.
func RunPreUpdateHooks(ctx context.Context, hooks []Hook, resourceID string) error {
	return runPreHooks(ctx, hooks, resourceID, "pre-update")
}

// RunPostInstallHooks executes non-deferred post-install hooks.
func RunPostInstallHooks(ctx context.Context, hooks []Hook, resourceID string, sf *state.File) error {
	return runPostHooks(ctx, hooks, resourceID, "post-install", sf)
}

// RunPostUpdateHooks executes non-deferred post-update hooks.
func RunPostUpdateHooks(ctx context.Context, hooks []Hook, resourceID string, sf *state.File) error {
	return runPostHooks(ctx, hooks, resourceID, "post-update", sf)
}

// runPreHooks runs non-deferred pre-phase hooks. Returns error on first failure.
func runPreHooks(ctx context.Context, hooks []Hook, resourceID, phase string) error {
	for _, h := range hooks {
		if h.Defer {
			continue
		}
		if err := runHook(ctx, h, resourceID); err != nil {
			return fmt.Errorf("%s hook for %s failed: %w", phase, resourceID, err)
		}
	}
	return nil
}

// runPostHooks runs non-deferred post-phase hooks. Records hook-failed state on error.
func runPostHooks(ctx context.Context, hooks []Hook, resourceID, phase string, sf *state.File) error {
	for _, h := range hooks {
		if h.Defer {
			continue
		}
		if err := runHook(ctx, h, resourceID); err != nil {
			slog.Error(phase+" hook failed", "resource", resourceID, "error", err)
			sf.SetResource(resourceID, state.StateHookFailed, state.WithError(err.Error()))
			return fmt.Errorf("%s hook for %s failed: %w", phase, resourceID, err)
		}
	}
	return nil
}

// CollectDeferredHooks returns all hooks with defer=true from the given hook set.
func CollectDeferredHooks(resourceID string, hooks []Hook) []DeferredHook {
	var deferred []DeferredHook
	for _, h := range hooks {
		if h.Defer {
			deferred = append(deferred, DeferredHook{Hook: h, ResourceID: resourceID})
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

	// Nested hams invocations run as subprocess; in-process dispatch not yet supported.
	if strings.HasPrefix(strings.TrimSpace(h.Command), "hams ") {
		slog.Warn("nested hams invocation detected in hook; running as subprocess",
			"resource", resourceID, "command", h.Command)
	}

	// Cycle 178: tee stdout/stderr to the user's terminal AND a buffer.
	// Pre-cycle-178 used CombinedOutput which silenced everything until
	// the hook finished — long-running hooks (compilation, network
	// calls, brew bottle install) appeared to hang for minutes with
	// no progress indication. Now: streams to terminal so the user
	// sees output in real time, AND captures into a buffer so the
	// error path can include the output for debugging.
	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command) //nolint:gosec // hook commands are user-defined in hamsfile, not external input
	var captured bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &captured)
	cmd.Stderr = io.MultiWriter(os.Stderr, &captured)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q: %w\noutput: %s", h.Command, err, captured.String())
	}
	return nil
}
