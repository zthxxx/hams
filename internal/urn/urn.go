// Package urn implements parsing, validation, and formatting of urn:hams resource identifiers.
package urn

import (
	"fmt"
	"strings"
)

// URN represents a parsed hams resource identifier
// in the format urn:hams:<provider>:<resource-id>.
type URN struct {
	Provider   string
	ResourceID string
}

// Parse parses a URN string into a URN struct.
// The input must be in the format "urn:hams:<provider>:<resource-id>".
func Parse(s string) (URN, error) {
	if s == "" {
		return URN{}, fmt.Errorf("urn: empty string")
	}

	parts := strings.SplitN(s, ":", 4)
	if len(parts) != 4 {
		return URN{}, fmt.Errorf("urn: expected 4 colon-separated parts, got %d in %q", len(parts), s)
	}

	if parts[0] != "urn" {
		return URN{}, fmt.Errorf("urn: must start with 'urn:', got %q", s)
	}
	if parts[1] != "hams" {
		return URN{}, fmt.Errorf("urn: namespace must be 'hams', got %q in %q", parts[1], s)
	}
	if parts[2] == "" {
		return URN{}, fmt.Errorf("urn: provider must not be empty in %q", s)
	}
	if parts[3] == "" {
		return URN{}, fmt.Errorf("urn: resource-id must not be empty in %q", s)
	}
	if strings.Contains(parts[3], ":") {
		return URN{}, fmt.Errorf("urn: resource-id must not contain colons in %q", s)
	}

	provider := strings.ToLower(parts[2])
	resourceID := parts[3]

	return URN{
		Provider:   provider,
		ResourceID: resourceID,
	}, nil
}

// String formats the URN as a canonical string.
func (u URN) String() string {
	return fmt.Sprintf("urn:hams:%s:%s", u.Provider, u.ResourceID)
}

// New creates a new URN from provider and resource-id components.
// It validates the components and returns an error if invalid.
func New(provider, resourceID string) (URN, error) {
	if provider == "" {
		return URN{}, fmt.Errorf("urn: provider must not be empty")
	}
	if resourceID == "" {
		return URN{}, fmt.Errorf("urn: resource-id must not be empty")
	}
	if strings.Contains(resourceID, ":") {
		return URN{}, fmt.Errorf("urn: resource-id must not contain colons: %q", resourceID)
	}

	return URN{
		Provider:   strings.ToLower(provider),
		ResourceID: resourceID,
	}, nil
}

// IsValid returns true if the URN has non-empty provider and resource-id.
func (u URN) IsValid() bool {
	return u.Provider != "" && u.ResourceID != "" && !strings.Contains(u.ResourceID, ":")
}
