// Package hamsfile provides comment-preserving YAML read/write for Hamsfile store files.
package hamsfile

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	fieldApp = "app"
	fieldURN = "urn"
)

// File represents a loaded Hamsfile with its raw YAML node tree
// for comment-preserving round-trip editing.
type File struct {
	Path string
	Root *yaml.Node
}

// mu protects all Hamsfile write operations (global write-serial lock).
var mu sync.Mutex

// Read loads a Hamsfile from disk, preserving the full YAML node tree
// including comments, ordering, and formatting.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path) //nolint:gosec // hamsfile paths are user-configured store locations
	if err != nil {
		return nil, fmt.Errorf("reading hamsfile %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing hamsfile %s: %w", path, err)
	}

	return &File{Path: path, Root: &root}, nil
}

// Write saves the Hamsfile back to disk atomically, preserving comments.
// This operation acquires the global write lock.
func (f *File) Write() error {
	mu.Lock()
	defer mu.Unlock()

	data, err := yaml.Marshal(f.Root)
	if err != nil {
		return fmt.Errorf("marshaling hamsfile: %w", err)
	}

	return atomicWrite(f.Path, data)
}

// Tags returns all top-level tag names (category keys) in the Hamsfile.
func (f *File) Tags() []string {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return nil
	}

	// yaml.Unmarshal wraps in a Document node.
	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}

	if doc.Kind != yaml.MappingNode {
		return nil
	}

	var tags []string
	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Kind == yaml.ScalarNode {
			tags = append(tags, doc.Content[i].Value)
		}
	}
	return tags
}

// ListApps returns all app/URN names from all tags in the Hamsfile.
func (f *File) ListApps() []string {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return nil
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}

	if doc.Kind != yaml.MappingNode {
		return nil
	}

	var apps []string
	for i := 0; i < len(doc.Content)-1; i += 2 {
		valNode := doc.Content[i+1]
		if valNode.Kind != yaml.SequenceNode {
			continue
		}

		for _, item := range valNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k < len(item.Content)-1; k += 2 {
				key := item.Content[k].Value
				if key == fieldApp || key == fieldURN {
					apps = append(apps, item.Content[k+1].Value)
					break
				}
			}
		}
	}
	return apps
}

// FindApp searches all tags for a package entry with the given app name.
// Returns the tag name and index within the tag's sequence, or -1 if not found.
func (f *File) FindApp(appName string) (tag string, index int) {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return "", -1
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}

	if doc.Kind != yaml.MappingNode {
		return "", -1
	}

	for i := 0; i < len(doc.Content)-1; i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode || valNode.Kind != yaml.SequenceNode {
			continue
		}

		for j, item := range valNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k < len(item.Content)-1; k += 2 {
				if item.Content[k].Value == fieldApp && item.Content[k+1].Value == appName {
					return keyNode.Value, j
				}
			}
		}
	}

	return "", -1
}

// AddApp adds a package entry under the specified tag.
// If the tag doesn't exist, it creates a new top-level section.
func (f *File) AddApp(tag, appName, intro string) {
	mu.Lock()
	defer mu.Unlock()

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}

	// Build the new app entry node.
	entry := buildAppEntry(appName, intro)

	// Find or create the tag section.
	if doc.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Kind == yaml.ScalarNode && doc.Content[i].Value == tag {
			// Tag exists — append to its sequence.
			seq := doc.Content[i+1]
			if seq.Kind == yaml.SequenceNode {
				seq.Content = append(seq.Content, entry)
			}
			return
		}
	}

	// Tag doesn't exist — create it.
	tagKey := &yaml.Node{Kind: yaml.ScalarNode, Value: tag, Tag: "!!str"}
	tagSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{entry}}
	doc.Content = append(doc.Content, tagKey, tagSeq)
}

// RemoveApp removes a package entry by app name from all tags.
// Returns true if the entry was found and removed.
func (f *File) RemoveApp(appName string) bool {
	mu.Lock()
	defer mu.Unlock()

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}

	if doc.Kind != yaml.MappingNode {
		return false
	}

	found := false
	for i := 0; i < len(doc.Content)-1; i += 2 {
		valNode := doc.Content[i+1]
		if valNode.Kind != yaml.SequenceNode {
			continue
		}

		for j := len(valNode.Content) - 1; j >= 0; j-- {
			item := valNode.Content[j]
			if item.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k < len(item.Content)-1; k += 2 {
				if item.Content[k].Value == fieldApp && item.Content[k+1].Value == appName {
					valNode.Content = append(valNode.Content[:j], valNode.Content[j+1:]...)
					found = true
				}
			}
		}
	}

	return found
}

func buildAppEntry(appName, intro string) *yaml.Node {
	content := []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "app", Tag: "!!str"},
		{Kind: yaml.ScalarNode, Value: appName, Tag: "!!str"},
	}
	if intro != "" {
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "intro", Tag: "!!str"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: intro, Tag: "!!str"},
		)
	}
	return &yaml.Node{Kind: yaml.MappingNode, Content: content}
}
