package hamsfile

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReadMerged_NoLocal(t *testing.T) {
	mainPath := writeTempFile(t, sampleYAML)
	localPath := mainPath + ".nonexistent"

	f, err := ReadMerged(mainPath, localPath, MergeAppend)
	if err != nil {
		t.Fatalf("ReadMerged error: %v", err)
	}

	tags := f.Tags()
	if len(tags) != 2 {
		t.Errorf("Tags() = %v, want 2 tags", tags)
	}
}

func TestReadMerged_AppendStrategy(t *testing.T) {
	mainContent := `development-tool:
  - app: htop
    intro: Top replacement
`
	localContent := `development-tool:
  - app: ripgrep
    intro: Fast search
network-tool:
  - app: curl
    intro: Transfer data
`
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "Homebrew.hams.yaml")
	localPath := filepath.Join(dir, "Homebrew.hams.local.yaml")
	writeFile(t, mainPath, mainContent)
	writeFile(t, localPath, localContent)

	f, err := ReadMerged(mainPath, localPath, MergeAppend)
	if err != nil {
		t.Fatalf("ReadMerged error: %v", err)
	}

	// Should have htop + ripgrep under development-tool.
	tag, idx := f.FindApp("htop")
	if tag != "development-tool" || idx < 0 {
		t.Errorf("htop: tag=%q, idx=%d", tag, idx)
	}
	tag, idx = f.FindApp("ripgrep")
	if tag != "development-tool" || idx < 0 {
		t.Errorf("ripgrep: tag=%q, idx=%d", tag, idx)
	}

	// network-tool should be appended as new tag.
	tag, idx = f.FindApp("curl")
	if tag != "network-tool" || idx < 0 {
		t.Errorf("curl: tag=%q, idx=%d", tag, idx)
	}
}

func TestReadMerged_OverrideStrategy(t *testing.T) {
	mainContent := `configs:
  - urn: "urn:hams:defaults:dock.autohide"
    args:
      value: "false"
  - urn: "urn:hams:defaults:dock.mineffect"
    args:
      value: "genie"
`
	localContent := `configs:
  - urn: "urn:hams:defaults:dock.autohide"
    args:
      value: "true"
`
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "defaults.hams.yaml")
	localPath := filepath.Join(dir, "defaults.hams.local.yaml")
	writeFile(t, mainPath, mainContent)
	writeFile(t, localPath, localContent)

	f, err := ReadMerged(mainPath, localPath, MergeOverride)
	if err != nil {
		t.Fatalf("ReadMerged error: %v", err)
	}

	// Verify the overridden entry.
	doc := documentContent(f.Root)
	if doc.Kind != 4 { // MappingNode
		t.Fatalf("unexpected root kind: %d", doc.Kind)
	}

	// Find the configs sequence.
	seqIdx := findMappingKey(doc, "configs")
	if seqIdx < 0 {
		t.Fatal("configs key not found")
	}
	seq := doc.Content[seqIdx+1]

	// Should have 2 entries (one overridden, one unchanged).
	if len(seq.Content) != 2 {
		t.Fatalf("expected 2 config entries, got %d", len(seq.Content))
	}

	// The autohide entry should be the overridden one (from local).
	autohideEntry := seq.Content[0]
	urnVal := findEntryField(autohideEntry, "urn")
	if urnVal != "urn:hams:defaults:dock.autohide" {
		t.Errorf("first entry URN = %q, want dock.autohide", urnVal)
	}
}

func TestReadMerged_OverrideAppendNew(t *testing.T) {
	mainContent := `configs:
  - urn: "urn:hams:defaults:existing"
    args:
      value: old
`
	localContent := `configs:
  - urn: "urn:hams:defaults:brand-new"
    args:
      value: new
`
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "defaults.hams.yaml")
	localPath := filepath.Join(dir, "defaults.hams.local.yaml")
	writeFile(t, mainPath, mainContent)
	writeFile(t, localPath, localContent)

	f, err := ReadMerged(mainPath, localPath, MergeOverride)
	if err != nil {
		t.Fatalf("ReadMerged error: %v", err)
	}

	doc := documentContent(f.Root)
	seqIdx := findMappingKey(doc, "configs")
	seq := doc.Content[seqIdx+1]

	// Should have 2 entries (existing + new).
	if len(seq.Content) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(seq.Content))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func findEntryField(node *yaml.Node, fieldName string) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == fieldName {
			return node.Content[i+1].Value
		}
	}
	return ""
}
