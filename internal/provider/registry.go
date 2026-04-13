package provider

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Registry holds all registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates a new empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
// Returns an error if a provider with the same name is already registered.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m := p.Manifest()
	name := strings.ToLower(m.Name)

	if existing, ok := r.providers[name]; ok {
		return fmt.Errorf("provider %q already registered (existing: %s, new: %s)",
			name, existing.Manifest().DisplayName, m.DisplayName)
	}

	// Check platform compatibility.
	if !isPlatformsMatch(m.Platforms) {
		return nil // Silently skip providers for other platforms.
	}

	r.providers[name] = p
	return nil
}

// Get returns a provider by name, or nil if not found.
func (r *Registry) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[strings.ToLower(name)]
}

// All returns all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// Names returns all registered provider names, sorted alphabetically.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Ordered returns providers sorted by the given priority list.
// Providers in the priority list come first (in that order).
// Remaining providers are appended alphabetically.
func (r *Registry) Ordered(priority []string) []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var result []Provider

	// Add providers in priority order.
	for _, name := range priority {
		name = strings.ToLower(name)
		if p, ok := r.providers[name]; ok {
			result = append(result, p)
			seen[name] = true
		}
	}

	// Add remaining providers alphabetically.
	var remaining []string
	for name := range r.providers {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)

	for _, name := range remaining {
		result = append(result, r.providers[name])
	}

	return result
}

func isPlatformsMatch(platforms []Platform) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == PlatformAll || p == "" || string(p) == runtime.GOOS {
			return true
		}
	}
	return false
}
