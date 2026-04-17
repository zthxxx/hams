package provider

import (
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
)

// ParseHookSet converts the YAML mapping node returned by
// hamsfile.File.AppHookNode into a *HookSet for action.Hooks
// population. Returns nil when the node is nil, the wrong YAML kind,
// or contains no recognized hook keys.
//
// Recognized keys: pre_install, post_install, pre_update, post_update.
// Each value must be a sequence of mapping items with at least a
// `run:` field. The `defer:` field is optional (default false).
//
// Unknown keys and non-mapping list items are silently skipped — the
// hamsfile may have v1.1 keys we don't yet recognize, and a
// permissive parser is friendlier than a strict one for users
// upgrading between hams versions.
func ParseHookSet(node *yaml.Node) *HookSet {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	hs := &HookSet{}
	for k := 0; k < len(node.Content)-1; k += 2 {
		key := node.Content[k].Value
		val := node.Content[k+1]
		if val == nil || val.Kind != yaml.SequenceNode {
			continue
		}
		switch key {
		case "pre_install":
			hs.PreInstall = parseHookList(val, HookPreInstall)
		case "post_install":
			hs.PostInstall = parseHookList(val, HookPostInstall)
		case "pre_update":
			hs.PreUpdate = parseHookList(val, HookPreUpdate)
		case "post_update":
			hs.PostUpdate = parseHookList(val, HookPostUpdate)
		}
	}

	if !hs.HasAny() {
		return nil
	}
	return hs
}

// HasAny reports whether the HookSet contains at least one Hook
// across any of the four phases.
func (hs *HookSet) HasAny() bool {
	if hs == nil {
		return false
	}
	return len(hs.PreInstall) > 0 ||
		len(hs.PostInstall) > 0 ||
		len(hs.PreUpdate) > 0 ||
		len(hs.PostUpdate) > 0
}

// parseHookList walks a YAML sequence of hook entries, producing one
// Hook per well-formed item. Items missing `run:` are skipped (a hook
// with an empty command is a config bug, not a usable Hook).
//
// Cycle 200: emits a slog.Warn for any hook with `defer: true`. The
// deferred-hooks feature (run all defer:true hooks AFTER every
// non-deferred action in the provider completes) has RunDeferredHooks
// and CollectDeferredHooks scaffolded in hooks.go but NO production
// caller wires them — same scaffolded-but-unwired pattern as
// --hams-lucky (cycle 3). The hook parses but never fires. Mirror
// cycle 3's fail-loud strategy so users notice instead of silently
// losing the hook.
func parseHookList(seq *yaml.Node, ht HookType) []Hook {
	var hooks []Hook
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		var h Hook
		h.Type = ht
		for k := 0; k < len(item.Content)-1; k += 2 {
			key := item.Content[k].Value
			val := item.Content[k+1].Value
			switch key {
			case "run":
				h.Command = strings.TrimSpace(val)
			case "defer":
				h.Defer = parseBoolLoose(val)
			}
		}
		if h.Command != "" {
			if h.Defer {
				slog.Warn("hook with `defer: true` parsed but not yet executed (deferred-hooks feature not wired in v1)",
					"hook_type", ht.String(), "command", h.Command)
			}
			hooks = append(hooks, h)
		}
	}
	return hooks
}

// parseBoolLoose accepts the YAML 1.1 boolean variants ("true",
// "yes", "on", "1") in addition to YAML 1.2's strict "true". The
// hamsfile parser's downstream consumers already accept loose
// booleans elsewhere; consistency matters more than spec strictness.
func parseBoolLoose(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// PopulateActionHooks walks each action and, when desired contains a
// `hooks:` block for that action's resource ID, attaches the parsed
// HookSet to action.Hooks. Returns the same actions slice for fluent
// chaining.
//
// Most providers' Plan() implementation now ends with:
//
//	return provider.PopulateActionHooks(actions, desired), nil
//
// which is enough to wire hamsfile-declared hooks through to the
// executor's pre/post hook dispatch (executor.go:100-150).
func PopulateActionHooks(actions []Action, desired *hamsfile.File) []Action {
	if desired == nil {
		return actions
	}
	for i := range actions {
		node := desired.AppHookNode(actions[i].ID)
		if node == nil {
			continue
		}
		if hs := ParseHookSet(node); hs != nil {
			actions[i].Hooks = hs
		}
	}
	return actions
}
