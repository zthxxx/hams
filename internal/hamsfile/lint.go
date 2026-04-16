package hamsfile

import (
	"gopkg.in/yaml.v3"
)

// Hooks key constants. The full v1.1 wiring will live in a dedicated
// hooks package; for v1, these are recognized only so the loader can
// emit a single deferred-feature warning when a user's hamsfile
// declares them.
const (
	fieldHooks       = "hooks"
	hookKeyPreInst   = "pre_install"
	hookKeyPostInst  = "post_install"
	hookKeyPreUpdate = "pre_update"
	hookKeyPostUpd   = "post_update"
)

// DeferredFeatures lists the v1.1-deferred feature usages found in
// the loaded Hamsfile. Empty when none are present. Callers (the
// `hams apply` entry point in particular) emit a slog.Warn so users
// who copied the documented `hooks:` example are not surprised by a
// silent no-op.
//
// The current v1 spec status is documented at:
//
//	openspec/specs/schema-design/spec.md   (Hamsfile Hooks Schema)
//	openspec/specs/cli-architecture/spec.md (hooks-defer + OTel-defer deltas)
//
// The execution engine itself is built and tested
// (internal/provider/hooks.go) — only the parser-to-Action wiring
// remains. v1.1 will populate Action.Hooks and remove this lint.
type DeferredFeatures struct {
	// HookEntries lists the app/urn IDs that declared a `hooks:`
	// block. Empty when none are present.
	HookEntries []string
}

// HasAny reports whether any deferred feature was found.
func (d DeferredFeatures) HasAny() bool {
	return len(d.HookEntries) > 0
}

// LintDeferredFeatures scans the loaded hamsfile for v1.1-deferred
// feature usages. Currently checks for `hooks:` blocks under any item.
// Returns the list of affected app/urn IDs so the caller can name
// them in a warning.
//
// Implementation: walks the same node tree as ListApps but additionally
// checks each item's mapping content for a `hooks:` key. Whitespace-
// or tab-only hook nodes are still flagged — the warning is opt-in for
// users; precision over false-positives matters less than visibility.
func LintDeferredFeatures(f *File) DeferredFeatures {
	doc := f.DocMapping()
	if doc == nil {
		return DeferredFeatures{}
	}

	var hooked []string
	for i := 0; i < len(doc.Content)-1; i += 2 {
		valNode := doc.Content[i+1]
		if valNode.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range valNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			id, hasHooks := scanItemForHooks(item)
			if hasHooks && id != "" {
				hooked = append(hooked, id)
			}
		}
	}
	return DeferredFeatures{HookEntries: hooked}
}

// scanItemForHooks walks an item-mapping's keys looking for the `app`
// or `urn` value and a `hooks:` key. Returns (id, hasHooks).
func scanItemForHooks(item *yaml.Node) (id string, hasHooks bool) {
	for k := 0; k < len(item.Content)-1; k += 2 {
		key := item.Content[k].Value
		switch key {
		case fieldApp, fieldURN:
			id = item.Content[k+1].Value
		case fieldHooks:
			hasHooks = isNonEmptyHookNode(item.Content[k+1])
		}
	}
	return id, hasHooks
}

// isNonEmptyHookNode reports whether a `hooks:` value contains at
// least one of the four recognized hook keys with a non-empty value.
// An empty mapping (`hooks: {}`) or a mapping with only blank values
// is NOT flagged — users who clear hooks shouldn't get a warning.
func isNonEmptyHookNode(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for k := 0; k < len(node.Content)-1; k += 2 {
		key := node.Content[k].Value
		val := node.Content[k+1]
		switch key {
		case hookKeyPreInst, hookKeyPostInst, hookKeyPreUpdate, hookKeyPostUpd:
			if val != nil && val.Kind == yaml.SequenceNode && len(val.Content) > 0 {
				return true
			}
		}
	}
	return false
}
