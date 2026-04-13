package provider

import (
	"fmt"
	"runtime"
	"strings"
)

// ResolveDAG performs a topological sort of providers based on their depend-on declarations.
// Returns providers in execution order (dependencies first).
// Returns an error if a cycle is detected.
func ResolveDAG(providers []Provider) ([]Provider, error) {
	byName := make(map[string]Provider)
	for _, p := range providers {
		byName[strings.ToLower(p.Manifest().Name)] = p
	}

	// Build adjacency list: edges go from dependency to dependent.
	// In-degree counts how many dependencies each provider has.
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dependency -> list of dependents

	for _, p := range providers {
		name := strings.ToLower(p.Manifest().Name)
		if _, ok := inDegree[name]; !ok {
			inDegree[name] = 0
		}

		for _, dep := range p.Manifest().DependsOn {
			if !matchesPlatform(dep.Platform) {
				continue
			}

			depName := strings.ToLower(dep.Provider)
			inDegree[name]++
			dependents[depName] = append(dependents[depName], name)

			if _, ok := inDegree[depName]; !ok {
				inDegree[depName] = 0
			}
		}
	}

	// Kahn's algorithm for topological sort.
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// If not all nodes are sorted, there is a cycle.
	if len(sorted) != len(inDegree) {
		cycle := findCycleProviders(inDegree, dependents)
		return nil, fmt.Errorf("provider dependency cycle detected: %s", strings.Join(cycle, " -> "))
	}

	// Map back to providers, preserving only those that were in the input.
	result := make([]Provider, 0, len(providers))
	for _, name := range sorted {
		if p, ok := byName[name]; ok {
			result = append(result, p)
		}
	}

	return result, nil
}

// findCycleProviders finds providers involved in a cycle (for error reporting).
func findCycleProviders(inDegree map[string]int, dependents map[string][]string) []string {
	// Collect nodes still with non-zero in-degree.
	var cycle []string
	for name, degree := range inDegree {
		if degree > 0 {
			cycle = append(cycle, name)
		}
	}

	// Try to reconstruct a cycle path from the first node.
	if len(cycle) > 0 {
		visited := make(map[string]bool)
		path := []string{cycle[0]}
		current := cycle[0]
		visited[current] = true

		for {
			found := false
			for _, next := range dependents[current] {
				if inDegree[next] <= 0 {
					continue
				}
				if visited[next] {
					path = append(path, next)
					return path
				}
				visited[next] = true
				path = append(path, next)
				current = next
				found = true
				break
			}
			if !found {
				break
			}
		}
		return path
	}

	return cycle
}

func matchesPlatform(p Platform) bool {
	if p == "" || p == PlatformAll {
		return true
	}
	return string(p) == runtime.GOOS
}
