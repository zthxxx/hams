package hamsfile

import (
	"gopkg.in/yaml.v3"
)

// fieldHooks is the YAML key for lifecycle hooks on an item.
const fieldHooks = "hooks"

// AppHookNode returns the YAML mapping node for the `hooks:` key of
// the item whose `app:` or `urn:` value matches appID. Returns nil
// when:
//
//   - the file is empty,
//   - no item matches appID,
//   - the matching item has no `hooks:` key, or
//   - the `hooks:` value is not a mapping.
//
// The provider package's ParseHookSet consumes this node and converts
// it to a *provider.HookSet for action.Hooks population.
//
// Implementation note: the walk mirrors ListApps so the matching
// rules are identical (only ScalarNode keys, only SequenceNode
// values, only MappingNode items with `app:` or `urn:`).
func (f *File) AppHookNode(appID string) *yaml.Node {
	doc := f.DocMapping()
	if doc == nil {
		return nil
	}

	for i := 0; i < len(doc.Content)-1; i += 2 {
		valNode := doc.Content[i+1]
		if valNode.Kind != yaml.SequenceNode {
			continue
		}

		for _, item := range valNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			id, hookNode := scanItemForHookNode(item)
			if id == appID && hookNode != nil {
				return hookNode
			}
		}
	}
	return nil
}

// scanItemForHookNode walks an item-mapping's keys, returning the
// app/urn ID and the value node of `hooks:` (or nil if absent).
func scanItemForHookNode(item *yaml.Node) (id string, hookNode *yaml.Node) {
	for k := 0; k < len(item.Content)-1; k += 2 {
		key := item.Content[k].Value
		switch key {
		case fieldApp, fieldURN:
			id = item.Content[k+1].Value
		case fieldHooks:
			hookNode = item.Content[k+1]
		}
	}
	return id, hookNode
}
