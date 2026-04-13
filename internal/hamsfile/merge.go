package hamsfile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MergeStrategy defines how a provider's .local.yaml merges with the main file.
type MergeStrategy int

const (
	// MergeAppend adds local entries after main entries within each tag.
	// Used by package-list providers (homebrew, pnpm, npm, etc.).
	MergeAppend MergeStrategy = iota
	// MergeOverride replaces entries with the same key (URN or app name).
	// Used by URN-keyed providers (bash, defaults, duti, git config).
	MergeOverride
)

// ReadMerged loads a Hamsfile and its .local.yaml counterpart, merging them.
// The main file is loaded first, then the local file is merged on top.
// If the local file doesn't exist, only the main file is returned.
func ReadMerged(mainPath, localPath string, strategy MergeStrategy) (*File, error) {
	main, err := Read(mainPath)
	if err != nil {
		return nil, err
	}

	_, statErr := os.Stat(localPath)
	if os.IsNotExist(statErr) {
		return main, nil
	}

	local, err := Read(localPath)
	if err != nil {
		return nil, fmt.Errorf("reading local hamsfile %s: %w", localPath, err)
	}

	merged, err := merge(main, local, strategy)
	if err != nil {
		return nil, fmt.Errorf("merging %s with %s: %w", mainPath, localPath, err)
	}

	return merged, nil
}

func merge(main, local *File, strategy MergeStrategy) (*File, error) { //nolint:unparam // error return reserved for future validation
	mainDoc := documentContent(main.Root)
	localDoc := documentContent(local.Root)

	if mainDoc == nil || localDoc == nil {
		return main, nil
	}

	if mainDoc.Kind != yaml.MappingNode || localDoc.Kind != yaml.MappingNode {
		return main, nil
	}

	// Iterate over local's top-level keys (tags).
	for i := 0; i < len(localDoc.Content)-1; i += 2 {
		localKey := localDoc.Content[i]
		localVal := localDoc.Content[i+1]

		if localKey.Kind != yaml.ScalarNode {
			continue
		}

		// Find matching tag in main.
		mainIdx := findMappingKey(mainDoc, localKey.Value)

		if mainIdx < 0 {
			// Tag not in main — append the entire tag section.
			mainDoc.Content = append(mainDoc.Content, localKey, localVal)
			continue
		}

		mainVal := mainDoc.Content[mainIdx+1]
		if mainVal.Kind != yaml.SequenceNode || localVal.Kind != yaml.SequenceNode {
			continue
		}

		switch strategy {
		case MergeAppend:
			mainVal.Content = append(mainVal.Content, localVal.Content...)
		case MergeOverride:
			mergeOverrideSequence(mainVal, localVal)
		}
	}

	return main, nil
}

// mergeOverrideSequence merges local sequence into main by matching entries.
// Entries with the same "app" or "urn" key are replaced; new entries are appended.
func mergeOverrideSequence(main, local *yaml.Node) {
	for _, localItem := range local.Content {
		localID := extractEntryID(localItem)
		if localID == "" {
			main.Content = append(main.Content, localItem)
			continue
		}

		found := false
		for j, mainItem := range main.Content {
			if extractEntryID(mainItem) == localID {
				main.Content[j] = localItem
				found = true
				break
			}
		}

		if !found {
			main.Content = append(main.Content, localItem)
		}
	}
}

// extractEntryID gets the "app" or "urn" value from a mapping node.
func extractEntryID(node *yaml.Node) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		if key == fieldApp || key == fieldURN {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func documentContent(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func findMappingKey(mapping *yaml.Node, key string) int {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}
