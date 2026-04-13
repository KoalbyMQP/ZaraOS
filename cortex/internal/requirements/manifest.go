package requirements

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Manifest is the top-level requirements file structure.
type Manifest struct {
	Version  int       `json:"version"`
	Packages []Package `json:"packages"`
}

// Package describes a single managed container package.
type Package struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Version      string            `json:"version"`
	Essential    bool              `json:"essential"`
	Autostart    bool              `json:"autostart"`
	Priority     int               `json:"priority"`
	Depends      []string          `json:"depends"`
	Env          map[string]string `json:"env,omitempty"`
	HealthCheck  *HealthCheck      `json:"health_check,omitempty"`
	RunCondition string            `json:"run_condition,omitempty"`
}

// HealthCheck configures automatic restart behaviour for a package.
type HealthCheck struct {
	Enabled          bool `json:"enabled"`
	RestartOnFailure bool `json:"restart_on_failure"`
	MaxRestarts      int  `json:"max_restarts"`
}

// LoadManifest reads and validates a requirements JSON file.
// If primaryPath doesn't exist it falls back to fallbackPath.
func LoadManifest(primaryPath, fallbackPath string) (*Manifest, error) {
	path := primaryPath
	data, err := os.ReadFile(path)
	if err != nil && fallbackPath != "" {
		path = fallbackPath
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &m, nil
}

// Validate checks the manifest for structural errors: duplicate names,
// missing dependency references, and circular dependencies.
func (m *Manifest) Validate() error {
	if m.Version < 1 {
		return fmt.Errorf("unsupported manifest version %d", m.Version)
	}

	names := make(map[string]bool, len(m.Packages))
	for _, pkg := range m.Packages {
		if pkg.Name == "" {
			return fmt.Errorf("package with empty name")
		}
		if pkg.Image == "" {
			return fmt.Errorf("package %q missing image", pkg.Name)
		}
		if pkg.Version == "" {
			return fmt.Errorf("package %q missing version", pkg.Name)
		}
		if names[pkg.Name] {
			return fmt.Errorf("duplicate package name %q", pkg.Name)
		}
		names[pkg.Name] = true
	}

	// Check all dependency references are valid.
	for _, pkg := range m.Packages {
		for _, dep := range pkg.Depends {
			if !names[dep] {
				return fmt.Errorf("package %q depends on unknown package %q", pkg.Name, dep)
			}
			if dep == pkg.Name {
				return fmt.Errorf("package %q depends on itself", pkg.Name)
			}
		}
	}

	// Check for circular dependencies.
	if err := detectCycle(m.Packages); err != nil {
		return err
	}

	return nil
}

// detectCycle uses DFS-based cycle detection across the dependency graph.
func detectCycle(pkgs []Package) error {
	depMap := make(map[string][]string, len(pkgs))
	for _, pkg := range pkgs {
		depMap[pkg.Name] = pkg.Depends
	}

	const (
		white = 0 // unvisited
		gray  = 1 // in-progress
		black = 2 // done
	)
	color := make(map[string]int, len(pkgs))

	var visit func(name string) error
	visit = func(name string) error {
		color[name] = gray
		for _, dep := range depMap[name] {
			switch color[dep] {
			case gray:
				return fmt.Errorf("circular dependency: %s -> %s", name, dep)
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[name] = black
		return nil
	}

	for _, pkg := range pkgs {
		if color[pkg.Name] == white {
			if err := visit(pkg.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// AutostartPackages returns only the packages with autostart enabled.
func (m *Manifest) AutostartPackages() []Package {
	var out []Package
	for _, pkg := range m.Packages {
		if pkg.Autostart {
			out = append(out, pkg)
		}
	}
	return out
}

// GetPackage returns a package by name, or false if not found.
func (m *Manifest) GetPackage(name string) (Package, bool) {
	for _, pkg := range m.Packages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return Package{}, false
}

// Save writes the manifest to disk as formatted JSON.
func (m *Manifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// EvalRunCondition checks whether a package's run_condition is satisfied.
// Supported conditions:
//
//	""                        → always true
//	"file_exists:/some/path"  → true if file exists
//	"!file_exists:/some/path" → true if file does NOT exist
func EvalRunCondition(cond string) bool {
	if cond == "" {
		return true
	}

	negate := false
	if strings.HasPrefix(cond, "!") {
		negate = true
		cond = cond[1:]
	}

	if strings.HasPrefix(cond, "file_exists:") {
		path := strings.TrimPrefix(cond, "file_exists:")
		_, err := os.Stat(path)
		exists := err == nil
		if negate {
			return !exists
		}
		return exists
	}

	// Unknown condition type — treat as satisfied to be safe.
	return true
}

// ImageRef returns the full image reference with tag for a package.
func (p *Package) ImageRef(resolvedVersion string) string {
	tag := resolvedVersion
	if tag == "" {
		tag = p.Version
	}
	return p.Image + ":" + tag
}
