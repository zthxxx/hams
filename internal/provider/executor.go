package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zthxxx/hams/internal/otel"
	"github.com/zthxxx/hams/internal/state"
)

// ExecuteResult summarizes the outcome of an apply execution.
type ExecuteResult struct {
	Installed int
	Updated   int
	Removed   int
	Skipped   int
	Failed    int
	Errors    []error
}

// Execute runs actions sequentially for a single provider.
// It updates the state file after each action. If otelSession is non-nil, provider
// and resource-level spans are recorded.
func Execute(ctx context.Context, p Provider, actions []Action, sf *state.File, otelSession ...*otel.Session) ExecuteResult {
	var result ExecuteResult
	var session *otel.Session
	if len(otelSession) > 0 {
		session = otelSession[0]
	}

	name := p.Manifest().Name

	// Provider-level span.
	var providerSpan *otel.Span
	if session != nil {
		providerSpan = session.StartSpan("hams.provider."+name, "", map[string]string{
			"provider": name,
			"actions":  fmt.Sprintf("%d", len(actions)),
		})
	}

	for _, action := range actions {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			if session != nil && providerSpan != nil {
				session.EndSpan(providerSpan, "canceled")
			}
			return result
		default:
		}

		switch action.Type {
		case ActionSkip:
			result.Skipped++
			continue
		case ActionInstall:
			executeAction(ctx, p, action, sf, &result, session, phaseInstall)
		case ActionUpdate:
			executeAction(ctx, p, action, sf, &result, session, phaseUpdate)
		case ActionRemove:
			executeAction(ctx, p, action, sf, &result, session, phaseRemove)
		}
	}

	if session != nil && providerSpan != nil {
		status := "ok"
		if result.Failed > 0 {
			status = "error"
		}
		session.EndSpan(providerSpan, status)

		session.RecordMetric("hams.provider.failures", float64(result.Failed), "count", map[string]string{"provider": name})
		session.RecordMetric("hams.resources.total", float64(result.Installed+result.Updated+result.Removed+result.Skipped+result.Failed), "count", map[string]string{"provider": name})
	}

	return result
}

const (
	phaseInstall = "install"
	phaseUpdate  = "update"
	phaseRemove  = "remove"
)

// executeAction is the unified helper for install, update, and remove phases.
// The phase parameter must be one of phaseInstall, phaseUpdate, or phaseRemove.
func executeAction(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult, session *otel.Session, phase string) {
	name := p.Manifest().Name

	var span *otel.Span
	if session != nil {
		span = session.StartSpan("hams.resource."+phase, "", map[string]string{
			"provider": name, "resource": action.ID,
		})
	}

	// Set state to pending before executing (install only).
	if phase == phaseInstall {
		sf.SetResource(action.ID, state.StatePending)
	}

	// Run pre-hooks (install and update only).
	if err := runPhasePreHooks(ctx, action, phase); err != nil {
		slog.Error("pre-"+phase+" hook failed, skipping "+phase, "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: pre-%s hook %s: %w", name, phase, action.ID, err))
		endSpan(session, span, "error")
		return
	}

	// Execute the action.
	slog.Info(phase+"ing", "provider", name, "resource", action.ID)
	var execErr error
	if phase == phaseRemove {
		execErr = p.Remove(ctx, action.ID)
	} else {
		execErr = p.Apply(ctx, action)
	}
	if execErr != nil {
		slog.Error(phase+" failed", "provider", name, "resource", action.ID, "error", execErr)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(execErr.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: %s %s: %w", name, phase, action.ID, execErr))
		endSpan(session, span, "error")
		return
	}

	// Run post-hooks (install and update only).
	if err := runPhasePostHooks(ctx, action, phase, sf); err != nil {
		// Action succeeded but hook failed — state is hook-failed.
		incrementCounter(result, phase)
		result.Errors = append(result.Errors, fmt.Errorf("%s: post-%s hook %s: %w", name, phase, action.ID, err))
		slog.Info(phase+"d (hook failed)", "provider", name, "resource", action.ID)
		endSpan(session, span, "ok")
		return
	}

	// Success.
	if phase == phaseRemove {
		sf.SetResource(action.ID, state.StateRemoved)
	} else {
		sf.SetResource(action.ID, state.StateOK, action.StateOpts...)
	}
	incrementCounter(result, phase)
	slog.Info(phase+"d", "provider", name, "resource", action.ID)
	endSpan(session, span, "ok")
}

// runPhasePreHooks runs pre-hooks for the given phase. Returns nil if no hooks apply.
func runPhasePreHooks(ctx context.Context, action Action, phase string) error {
	if action.Hooks == nil {
		return nil
	}
	switch phase {
	case phaseInstall:
		if len(action.Hooks.PreInstall) > 0 {
			return RunPreInstallHooks(ctx, action.Hooks.PreInstall, action.ID)
		}
	case phaseUpdate:
		if len(action.Hooks.PreUpdate) > 0 {
			return RunPreUpdateHooks(ctx, action.Hooks.PreUpdate, action.ID)
		}
	}
	return nil
}

// runPhasePostHooks runs post-hooks for the given phase. Returns nil if no hooks apply.
func runPhasePostHooks(ctx context.Context, action Action, phase string, sf *state.File) error {
	if action.Hooks == nil {
		return nil
	}
	switch phase {
	case phaseInstall:
		if len(action.Hooks.PostInstall) > 0 {
			return RunPostInstallHooks(ctx, action.Hooks.PostInstall, action.ID, sf)
		}
	case phaseUpdate:
		if len(action.Hooks.PostUpdate) > 0 {
			return RunPostUpdateHooks(ctx, action.Hooks.PostUpdate, action.ID, sf)
		}
	}
	return nil
}

// endSpan closes an OTel span if the session is active.
func endSpan(session *otel.Session, span *otel.Span, status string) {
	if session != nil {
		session.EndSpan(span, status)
	}
}

func incrementCounter(result *ExecuteResult, phase string) {
	switch phase {
	case phaseInstall:
		result.Installed++
	case phaseUpdate:
		result.Updated++
	case phaseRemove:
		result.Removed++
	}
}

// MergeResults combines multiple ExecuteResults into one.
func MergeResults(results []ExecuteResult) ExecuteResult {
	var merged ExecuteResult
	for _, r := range results {
		merged.Installed += r.Installed
		merged.Updated += r.Updated
		merged.Removed += r.Removed
		merged.Skipped += r.Skipped
		merged.Failed += r.Failed
		merged.Errors = append(merged.Errors, r.Errors...)
	}
	return merged
}
