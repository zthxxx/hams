// Package llm provides LLM subprocess integration for tag/intro enrichment.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Config holds LLM integration settings.
type Config struct {
	CLI     string // Path to LLM CLI tool (e.g., "claude", "codex").
	Timeout time.Duration
}

// TagRecommendation is the response from the LLM for tag suggestions.
type TagRecommendation struct {
	Tags  []string `json:"tags"`
	Intro string   `json:"intro"`
}

// Recommend asks the LLM for tag and intro suggestions for a package.
func Recommend(ctx context.Context, cfg Config, packageName, description string, existingTags []string) (*TagRecommendation, error) {
	if cfg.CLI == "" {
		return nil, fmt.Errorf("LLM CLI not configured; set llm_cli in hams.config.yaml")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := buildPrompt(packageName, description, existingTags)

	slog.Debug("calling LLM", "cli", cfg.CLI, "package", packageName)
	cmd := exec.CommandContext(ctx, cfg.CLI, "-p", prompt) //nolint:gosec // CLI path from user config
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return parseResponse(string(output))
}

func buildPrompt(packageName, description string, existingTags []string) string {
	tagsStr := "none"
	if len(existingTags) > 0 {
		tagsStr = strings.Join(existingTags, ", ")
	}

	return fmt.Sprintf(
		`You are categorizing a software package for a machine setup tool.

Package: %s
Description: %s
Existing categories: [%s]

Respond ONLY with a JSON object (no markdown, no explanation):
{"tags": ["tag1", "tag2"], "intro": "one-line description"}

Rules:
- Choose 1-3 tags from existing categories when they fit
- Create new kebab-case tags only when no existing tag applies
- The intro should be a concise one-line description (under 80 chars)
- If the description already exists, use it as-is for intro`,
		packageName, description, tagsStr,
	)
}

func parseResponse(output string) (*TagRecommendation, error) {
	// Try to extract JSON from the response (LLM might wrap in markdown).
	output = strings.TrimSpace(output)

	// Strip markdown code blocks if present.
	if strings.HasPrefix(output, "```") {
		lines := strings.Split(output, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		output = strings.Join(jsonLines, "\n")
	}

	var rec TagRecommendation
	if err := json.Unmarshal([]byte(output), &rec); err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w\nraw output: %s", err, output)
	}

	return &rec, nil
}
