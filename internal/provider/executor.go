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
				session.EndSpan(providerSpan, "cancelled")
			}
			return result
		default:
		}

		switch action.Type {
		case ActionSkip:
			result.Skipped++
			continue
		case ActionInstall:
			executeInstall(ctx, p, action, sf, &result, session)
		case ActionUpdate:
			executeUpdate(ctx, p, action, sf, &result, session)
		case ActionRemove:
			executeRemove(ctx, p, action, sf, &result, session)
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

func executeInstall(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult, session *otel.Session) {
	name := p.Manifest().Name

	var span *otel.Span
	if session != nil {
		span = session.StartSpan("hams.resource.install", "", map[string]string{
			"provider": name, "resource": action.ID,
		})
	}

	// Set state to pending before executing.
	sf.SetResource(action.ID, state.StatePending)

	// Run pre-install hooks (non-deferred).
	if action.Hooks != nil && len(action.Hooks.PreInstall) > 0 {
		if err := RunPreInstallHooks(ctx, action.Hooks.PreInstall, action.ID); err != nil {
			slog.Error("pre-install hook failed, skipping install", "provider", name, "resource", action.ID, "error", err)
			sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
			result.Failed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: pre-install hook %s: %w", name, action.ID, err))
			if session != nil {
				session.EndSpan(span, "error")
			}
			return
		}
	}

	slog.Info("installing", "provider", name, "resource", action.ID)
	if err := p.Apply(ctx, action); err != nil {
		slog.Error("install failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: install %s: %w", name, action.ID, err))
		if session != nil {
			session.EndSpan(span, "error")
		}
		return
	}

	// Run post-install hooks (non-deferred).
	if action.Hooks != nil && len(action.Hooks.PostInstall) > 0 {
		if err := RunPostInstallHooks(ctx, action.Hooks.PostInstall, action.ID, sf); err != nil {
			// Install succeeded but hook failed — state is hook-failed.
			result.Installed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: post-install hook %s: %w", name, action.ID, err))
			slog.Info("installed (hook failed)", "provider", name, "resource", action.ID)
			if session != nil {
				session.EndSpan(span, "ok")
			}
			return
		}
	}

	sf.SetResource(action.ID, state.StateOK, action.StateOpts...)
	result.Installed++
	slog.Info("installed", "provider", name, "resource", action.ID)
	if session != nil {
		session.EndSpan(span, "ok")
	}
}

func executeUpdate(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult, session *otel.Session) {
	name := p.Manifest().Name

	var span *otel.Span
	if session != nil {
		span = session.StartSpan("hams.resource.update", "", map[string]string{
			"provider": name, "resource": action.ID,
		})
	}

	// Run pre-update hooks (non-deferred).
	if action.Hooks != nil && len(action.Hooks.PreUpdate) > 0 {
		if err := RunPreUpdateHooks(ctx, action.Hooks.PreUpdate, action.ID); err != nil {
			slog.Error("pre-update hook failed, skipping update", "provider", name, "resource", action.ID, "error", err)
			sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
			result.Failed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: pre-update hook %s: %w", name, action.ID, err))
			if session != nil {
				session.EndSpan(span, "error")
			}
			return
		}
	}

	slog.Info("updating", "provider", name, "resource", action.ID)
	if err := p.Apply(ctx, action); err != nil {
		slog.Error("update failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: update %s: %w", name, action.ID, err))
		if session != nil {
			session.EndSpan(span, "error")
		}
		return
	}

	// Run post-update hooks (non-deferred).
	if action.Hooks != nil && len(action.Hooks.PostUpdate) > 0 {
		if err := RunPostUpdateHooks(ctx, action.Hooks.PostUpdate, action.ID, sf); err != nil {
			// Update succeeded but hook failed — state is hook-failed.
			result.Updated++
			result.Errors = append(result.Errors, fmt.Errorf("%s: post-update hook %s: %w", name, action.ID, err))
			slog.Info("updated (hook failed)", "provider", name, "resource", action.ID)
			if session != nil {
				session.EndSpan(span, "ok")
			}
			return
		}
	}

	sf.SetResource(action.ID, state.StateOK, action.StateOpts...)
	result.Updated++
	slog.Info("updated", "provider", name, "resource", action.ID)
	if session != nil {
		session.EndSpan(span, "ok")
	}
}

func executeRemove(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult, session *otel.Session) {
	name := p.Manifest().Name

	var span *otel.Span
	if session != nil {
		span = session.StartSpan("hams.resource.remove", "", map[string]string{
			"provider": name, "resource": action.ID,
		})
	}

	slog.Info("removing", "provider", name, "resource", action.ID)
	if err := p.Remove(ctx, action.ID); err != nil {
		slog.Error("remove failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: remove %s: %w", name, action.ID, err))
		if session != nil {
			session.EndSpan(span, "error")
		}
		return
	}

	sf.SetResource(action.ID, state.StateRemoved)
	result.Removed++
	slog.Info("removed", "provider", name, "resource", action.ID)
	if session != nil {
		session.EndSpan(span, "ok")
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
