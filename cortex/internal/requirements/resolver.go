package requirements

import (
	"fmt"
	"sort"
)

// Resolve takes a set of packages and returns them grouped into tiers
// using Kahn's topological sort algorithm. Each tier is a batch of
// packages whose dependencies are all satisfied by earlier tiers.
// Within a tier, packages are sorted by priority (ascending), then name.
//
// Packages whose RunCondition evaluates to false are excluded from the
// result but still count as "satisfied" dependencies (so downstream
// packages that depend on them are not blocked).
func Resolve(pkgs []Package) ([][]Package, error) {
	// Build lookup and adjacency structures.
	byName := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		byName[p.Name] = p
	}

	// Determine which packages are active (pass run_condition).
	active := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		active[p.Name] = EvalRunCondition(p.RunCondition)
	}

	// Build in-degree counts only for active packages.
	// Inactive packages are treated as already-satisfied.
	inDegree := make(map[string]int, len(pkgs))
	dependents := make(map[string][]string) // dep -> list of packages that depend on it

	for _, p := range pkgs {
		if !active[p.Name] {
			continue
		}
		count := 0
		for _, dep := range p.Depends {
			if active[dep] {
				// Only count active dependencies toward in-degree.
				count++
				dependents[dep] = append(dependents[dep], p.Name)
			}
			// Inactive deps are already "satisfied", don't count.
		}
		inDegree[p.Name] = count
	}

	// Kahn's algorithm: collect nodes with in-degree 0 into tiers.
	var tiers [][]Package
	resolved := make(map[string]bool)

	for {
		// Find all active packages with in-degree 0.
		var tier []Package
		for _, p := range pkgs {
			if !active[p.Name] || resolved[p.Name] {
				continue
			}
			if inDegree[p.Name] == 0 {
				tier = append(tier, p)
			}
		}

		if len(tier) == 0 {
			break
		}

		// Sort tier by priority (ascending), then name (alphabetical).
		sort.Slice(tier, func(i, j int) bool {
			if tier[i].Priority != tier[j].Priority {
				return tier[i].Priority < tier[j].Priority
			}
			return tier[i].Name < tier[j].Name
		})

		// Mark resolved, decrement in-degrees for dependents.
		for _, p := range tier {
			resolved[p.Name] = true
			for _, downstream := range dependents[p.Name] {
				inDegree[downstream]--
			}
		}

		tiers = append(tiers, tier)
	}

	// Check for unresolved active packages (indicates a cycle the
	// Validate() check might have missed, or a bug).
	for _, p := range pkgs {
		if active[p.Name] && !resolved[p.Name] {
			return nil, fmt.Errorf("unresolvable dependency for package %q (possible cycle)", p.Name)
		}
	}

	return tiers, nil
}
