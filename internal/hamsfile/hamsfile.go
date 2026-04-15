// Package hamsfile provides comment-preserving YAML read/write for Hamsfile store files.
package hamsfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	fieldApp        = "app"
	fieldURN        = "urn"
	fieldPreviewCmd = "preview-cmd"
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

// NewEmpty returns an in-memory empty Hamsfile rooted at path. The Root
// is a DocumentNode wrapping an empty MappingNode; callers persist with
// Write(). Useful when synthesizing a "no desired state" hamsfile for
// prune-orphan reconciliation paths in apply, or as the create-empty
// branch of LoadOrCreateEmpty.
func NewEmpty(path string) *File {
	return &File{
		Path: path,
		Root: &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map"},
			},
		},
	}
}

// LoadOrCreateEmpty reads a Hamsfile, returning an empty in-memory File
// rooted at path when the file does not exist on disk. Callers persist
// the result by calling Write(). errors.Is + fs.ErrNotExist is required
// because Read wraps the underlying PathError with %w; the legacy
// os.IsNotExist API does not traverse wrapped chains.
func LoadOrCreateEmpty(path string) (*File, error) {
	f, err := Read(path)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create profile dir for %s: %w", path, err)
	}

	return NewEmpty(path), nil
}

// DocMapping returns the top-level mapping node, unwrapping the document node if present.
// Returns nil if the file is empty or the root is not a mapping.
func (f *File) DocMapping() *yaml.Node {
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
	return doc
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

	return AtomicWrite(f.Path, data)
}

// Tags returns all top-level tag names (category keys) in the Hamsfile.
func (f *File) Tags() []string {
	doc := f.DocMapping()
	if doc == nil {
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
	doc := f.DocMapping()
	if doc == nil {
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
	doc := f.DocMapping()
	if doc == nil {
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
	f.AddAppWithFields(tag, appName, intro, nil)
}

// AddAppWithFields is the structured-fields variant of AddApp. Each
// non-empty key/value pair in `extra` is emitted as an additional scalar
// entry on the package's mapping node, in iteration order. Empty values
// are skipped so callers can pass `{"version": "", "source": ""}`
// without polluting the YAML for bare-name entries.
//
// Used by providers that record optional structured pins (e.g., apt's
// version + source). Tag handling is identical to AddApp: append to an
// existing tag's sequence, or create a new tag at the document root.
func (f *File) AddAppWithFields(tag, appName, intro string, extra map[string]string) {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
		return
	}

	entry := buildAppEntryWithFields(appName, intro, extra)

	for i := 0; i < len(doc.Content)-1; i += 2 {
		if doc.Content[i].Kind == yaml.ScalarNode && doc.Content[i].Value == tag {
			seq := doc.Content[i+1]
			if seq.Kind == yaml.SequenceNode {
				seq.Content = append(seq.Content, entry)
			}
			return
		}
	}

	tagKey := &yaml.Node{Kind: yaml.ScalarNode, Value: tag, Tag: "!!str"}
	tagSeq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{entry}}
	doc.Content = append(doc.Content, tagKey, tagSeq)
}

// RemoveApp removes a package entry by app name from all tags.
// Returns true if the entry was found and removed.
func (f *File) RemoveApp(appName string) bool {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
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

// SetPreviewCmd sets or updates the preview-cmd field on a resource entry.
// Searches by app or urn name across all tags.
func (f *File) SetPreviewCmd(resourceName, previewCmd string) {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
		return
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
			// Check if this entry matches the resource name.
			isMatch := false
			for k := 0; k < len(item.Content)-1; k += 2 {
				key := item.Content[k].Value
				if (key == fieldApp || key == fieldURN) && item.Content[k+1].Value == resourceName {
					isMatch = true
					break
				}
			}
			if !isMatch {
				continue
			}

			// Update or add preview-cmd field.
			for k := 0; k < len(item.Content)-1; k += 2 {
				if item.Content[k].Value == fieldPreviewCmd {
					item.Content[k+1].Value = previewCmd
					return
				}
			}
			// Field doesn't exist yet — add it.
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: fieldPreviewCmd, Tag: "!!str"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: previewCmd, Tag: "!!str"},
			)
			return
		}
	}
}

// extraFieldOrder is the canonical emission order for the `extra` map
// in buildAppEntryWithFields. Iteration over a Go map is unordered; we
// fix the order so YAML round-trips byte-deterministically. Adding new
// providers' structured fields means appending to this slice.
var extraFieldOrder = []string{"version", "source"}

func buildAppEntryWithFields(appName, intro string, extra map[string]string) *yaml.Node {
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
	for _, key := range extraFieldOrder {
		val, ok := extra[key]
		if !ok || val == "" {
			continue
		}
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: val, Tag: "!!str"},
		)
	}
	return &yaml.Node{Kind: yaml.MappingNode, Content: content}
}
