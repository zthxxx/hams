package llm

import (
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	prompt := buildPrompt("htop", "Interactive process viewer", []string{"terminal-tool", "dev-tool"})
	if prompt == "" {
		t.Fatal("prompt should not be empty")
	}
	if !contains(prompt, "htop") {
		t.Error("prompt should contain package name")
	}
	if !contains(prompt, "terminal-tool") {
		t.Error("prompt should contain existing tags")
	}
}

func TestBuildPrompt_NoExistingTags(t *testing.T) {
	prompt := buildPrompt("htop", "desc", nil)
	if !contains(prompt, "none") {
		t.Error("prompt should show 'none' when no existing tags")
	}
}

func TestParseResponse_ValidJSON(t *testing.T) {
	input := `{"tags": ["terminal-tool", "monitoring"], "intro": "Interactive process viewer"}`
	rec, err := parseResponse(input)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if len(rec.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 items", rec.Tags)
	}
	if rec.Intro != "Interactive process viewer" {
		t.Errorf("Intro = %q", rec.Intro)
	}
}

func TestParseResponse_MarkdownWrapped(t *testing.T) {
	input := "```json\n{\"tags\": [\"dev\"], \"intro\": \"test\"}\n```"
	rec, err := parseResponse(input)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if len(rec.Tags) != 1 || rec.Tags[0] != "dev" {
		t.Errorf("Tags = %v, want [dev]", rec.Tags)
	}
}

func TestParseResponse_Invalid(t *testing.T) {
	_, err := parseResponse("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && // avoid trivial matches
		len(s) >= len(substr) &&
		indexSubstring(s, substr) >= 0
}

func indexSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
