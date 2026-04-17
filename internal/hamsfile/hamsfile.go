// Package hamsfile provides comment-preserving YAML read/write for Hamsfile store files.
package hamsfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
// Empty-string and whitespace-only values are skipped — a malformed
// hamsfile entry like `- app: ""` (e.g., from a git merge conflict
// or manual editing) must not flow into the provider's install
// command, which would otherwise attempt `brew install ""` or
// `apt install ""` and fail with a cryptic downstream error.
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
					if name := strings.TrimSpace(item.Content[k+1].Value); name != "" {
						apps = append(apps, name)
					}
					break
				}
			}
		}
	}
	return apps
}

// DuplicateAppError identifies a resource whose `app` / `urn` name
// appears under more than one top-level tag in the Hamsfile. Per
// schema-design spec §"Duplicate app identity across groups is
// rejected" (cycle 255), this case is a validation failure: the
// user likely moved an entry between tags without deleting the
// original copy, and silently merging the two under one identity
// would make drift attribution meaningless. The error names the
// offending app and lists every tag it appears in, in original
// document order.
type DuplicateAppError struct {
	App  string
	Tags []string
}

// Error satisfies the error interface with a human-readable
// message naming the duplicate and the tags it appears in.
func (e *DuplicateAppError) Error() string {
	return fmt.Sprintf("duplicate app %q found in tags: %s",
		e.App, strings.Join(e.Tags, ", "))
}

// ValidateNoDuplicateApps walks every tag's sequence and fails with
// a *DuplicateAppError if any app/urn identity appears under two or
// more tags. First-occurrence order is preserved for the tag list
// so the error message is stable across runs. Returns nil on
// success (including the "zero apps" case). Cycle 255.
//
// Duplicates WITHIN a single tag are not rejected here — those
// fold into a single action via ComputePlan's dedup (the common
// case is a typo that the user can fix; rejecting them entirely
// would break hand-edited files mid-correction). Cross-tag
// duplicates are the dangerous case because state attribution
// (which tag "owns" the app) becomes undefined.
func (f *File) ValidateNoDuplicateApps() error {
	doc := f.DocMapping()
	if doc == nil {
		return nil
	}

	// tagsByApp preserves first-occurrence order of tags per app.
	tagsByApp := make(map[string][]string)
	// seenInTag deduplicates within a single tag so same-tag repeats
	// don't inflate the reported tag list.
	for i := 0; i < len(doc.Content)-1; i += 2 {
		tagName := doc.Content[i].Value
		valNode := doc.Content[i+1]
		if valNode.Kind != yaml.SequenceNode {
			continue
		}

		seenInTag := make(map[string]bool)
		for _, item := range valNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k < len(item.Content)-1; k += 2 {
				key := item.Content[k].Value
				if key != fieldApp && key != fieldURN {
					continue
				}
				name := strings.TrimSpace(item.Content[k+1].Value)
				if name == "" {
					break
				}
				if seenInTag[name] {
					break
				}
				seenInTag[name] = true
				tagsByApp[name] = append(tagsByApp[name], tagName)
				break
			}
		}
	}

	// Scan in original document order (iterate the doc again) so the
	// first-detected duplicate gets reported deterministically. Map
	// iteration is unordered; re-walking is cheap for realistic
	// Hamsfile sizes (< 1000 entries).
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
				if key != fieldApp && key != fieldURN {
					continue
				}
				name := strings.TrimSpace(item.Content[k+1].Value)
				if name == "" {
					break
				}
				if tags := tagsByApp[name]; len(tags) > 1 {
					return &DuplicateAppError{App: name, Tags: tags}
				}
				break
			}
		}
	}
	return nil
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

// AddAppWithFields ensures a package entry exists with the given
// structured fields. Semantics:
//
//   - If `appName` is NOT in any tag yet, append a new entry under
//     `tag` (creating the tag if needed). Each non-empty key/value
//     in `extra` is emitted in `extraFieldOrder` so YAML round-trips
//     deterministically. Empty values are skipped — bare-name
//     entries stay byte-identical.
//   - If `appName` already exists under SOME tag, MERGE the
//     non-empty extras into that existing entry's mapping node in
//     place. The entry is NOT moved across tags. Empty extras are
//     no-ops on the existing entry. This makes the helper idempotent
//     and lets the apt CLI upgrade a bare `{app: nginx}` to
//     `{app: nginx, version: "1.24.0"}` on a re-install with a pin.
func (f *File) AddAppWithFields(tag, appName, intro string, extra map[string]string) {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
		return
	}

	if existing := findAppEntry(doc, appName); existing != nil {
		mergeFieldsIntoEntry(existing, extra)
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

// AppFields returns the structured per-app fields (everything except
// `app` and `intro`) for the entry whose `app` value matches name.
// Returns nil when no entry matches OR when the entry has no extra
// fields. Read-side counterpart to AddAppWithFields; callers consult
// it to recover provider-specific pins (e.g., apt's version, source)
// from a hand-edited or restored hamsfile.
func (f *File) AppFields(appName string) map[string]string {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
		return nil
	}
	entry := findAppEntry(doc, appName)
	if entry == nil {
		return nil
	}
	out := map[string]string{}
	for k := 0; k < len(entry.Content)-1; k += 2 {
		key := entry.Content[k].Value
		val := entry.Content[k+1]
		if key == fieldApp || key == "intro" {
			continue
		}
		if val.Kind != yaml.ScalarNode {
			continue
		}
		out[key] = val.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// findAppEntry returns the mapping node of the entry whose `app`
// scalar matches appName, searching across all tags. Returns nil when
// no entry matches. Caller must hold the mutex.
func findAppEntry(doc *yaml.Node, appName string) *yaml.Node {
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
				if item.Content[k].Value == fieldApp && item.Content[k+1].Value == appName {
					return item
				}
			}
		}
	}
	return nil
}

// mergeFieldsIntoEntry walks `extraFieldOrder` and, for each non-empty
// value in `extra`, sets it on the entry's mapping node — overwriting
// an existing scalar with the same key, OR appending a new key/value
// pair when absent. Empty values are skipped. Caller must hold the
// mutex.
func mergeFieldsIntoEntry(entry *yaml.Node, extra map[string]string) {
	for _, key := range extraFieldOrder {
		val, ok := extra[key]
		if !ok || val == "" {
			continue
		}
		setOrAppendScalarPair(entry, key, val)
	}
}

func setOrAppendScalarPair(entry *yaml.Node, key, val string) {
	for i := 0; i < len(entry.Content)-1; i += 2 {
		if entry.Content[i].Kind == yaml.ScalarNode && entry.Content[i].Value == key {
			entry.Content[i+1].Value = val
			entry.Content[i+1].Tag = "!!str"
			return
		}
	}
	entry.Content = append(entry.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: val, Tag: "!!str"},
	)
}

// RemoveAppField clears a single key/value pair from an existing app
// entry's mapping node. Used by CLI handlers that need to UNPIN a
// previously-pinned resource (e.g. `hams apt install nginx` after a
// prior `hams apt install nginx=1.24.0` must clear the version field
// — `AddAppWithFields` SKIPS empty values for the merge case so it
// can't itself express "remove this key", which would otherwise leave
// the hamsfile drifting from the actual install).
//
// Returns true when the field was present and removed; false when the
// entry doesn't exist or the field is absent. Idempotent — safe to
// call when the user re-runs a bare install with no prior pin.
func (f *File) RemoveAppField(appName, key string) bool {
	mu.Lock()
	defer mu.Unlock()

	doc := f.DocMapping()
	if doc == nil {
		return false
	}
	entry := findAppEntry(doc, appName)
	if entry == nil {
		return false
	}
	for i := 0; i < len(entry.Content)-1; i += 2 {
		if entry.Content[i].Kind == yaml.ScalarNode && entry.Content[i].Value == key {
			entry.Content = append(entry.Content[:i], entry.Content[i+2:]...)
			return true
		}
	}
	return false
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
