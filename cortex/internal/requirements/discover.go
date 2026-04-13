package requirements

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Well-known path inside every ZaraOS container image where the
// per-package manifest is embedded during the Docker build.
const embeddedManifestPath = "/etc/zaraos/package.json"

// PackageMeta is the per-package manifest format (zaraos.json).
// It omits "version" (comes from the registry/release tag) and
// "priority" (auto-calculated from dependency graph depth).
type PackageMeta struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Essential    bool              `json:"essential"`
	Autostart    bool              `json:"autostart"`
	Depends      []string          `json:"depends"`
	Env          map[string]string `json:"env,omitempty"`
	HealthCheck  *HealthCheck      `json:"health_check,omitempty"`
	RunCondition string            `json:"run_condition,omitempty"`
}

// ReadPackageMeta extracts the zaraos.json from a container image.
// It runs a temporary container that cats the file and parses the output.
func ReadPackageMeta(imageRef string) (*PackageMeta, error) {
	out, err := exec.Command(
		containerCLI(), "run", "--rm", "--entrypoint", "",
		imageRef, "cat", embeddedManifestPath,
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"read %s from %s: %s — %w",
			embeddedManifestPath, imageRef, strings.TrimSpace(string(out)), err,
		)
	}

	var meta PackageMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("parse package meta from %s: %w", imageRef, err)
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("package meta in %s has empty name", imageRef)
	}
	return &meta, nil
}

// ToPackage converts a PackageMeta to a full Package with the given version.
// Priority is left at 0 — the caller must run RecalcPriorities() on the
// manifest after merging to set it from the dependency graph.
func (pm *PackageMeta) ToPackage(version string) Package {
	if version == "" {
		version = "latest"
	}
	return Package{
		Name:         pm.Name,
		Image:        pm.Image,
		Version:      version,
		Essential:    pm.Essential,
		Autostart:    pm.Autostart,
		Priority:     0, // auto-calculated by RecalcPriorities
		Depends:      pm.Depends,
		Env:          pm.Env,
		HealthCheck:  pm.HealthCheck,
		RunCondition: pm.RunCondition,
	}
}

// MergePackage adds or updates a package in the manifest. If a package
// with the same name already exists, it is replaced. Otherwise it is appended.
// Priorities are recalculated from the dependency graph and the manifest
// is re-validated after the merge.
func (m *Manifest) MergePackage(pkg Package) error {
	found := false
	for i := range m.Packages {
		if m.Packages[i].Name == pkg.Name {
			m.Packages[i] = pkg
			found = true
			break
		}
	}
	if !found {
		m.Packages = append(m.Packages, pkg)
	}
	m.RecalcPriorities()
	return m.Validate()
}

// MergeAll replaces or appends every package from the given list.
// Priorities are recalculated and the manifest is re-validated.
func (m *Manifest) MergeAll(pkgs []Package) error {
	for _, pkg := range pkgs {
		found := false
		for i := range m.Packages {
			if m.Packages[i].Name == pkg.Name {
				m.Packages[i] = pkg
				found = true
				break
			}
		}
		if !found {
			m.Packages = append(m.Packages, pkg)
		}
	}
	m.RecalcPriorities()
	return m.Validate()
}

// MergeMetaFiles reads a list of zaraos.json file paths from disk and
// merges them all into the manifest. This is used at build time to
// generate the default requirements.json from per-package files.
func (m *Manifest) MergeMetaFiles(paths []string) error {
	for _, path := range paths {
		data, err := readFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var meta PackageMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		pkg := meta.ToPackage("latest")
		found := false
		for i := range m.Packages {
			if m.Packages[i].Name == pkg.Name {
				m.Packages[i] = pkg
				found = true
				break
			}
		}
		if !found {
			m.Packages = append(m.Packages, pkg)
		}
	}
	return m.Validate()
}

// readFile is a thin wrapper so it can be swapped in tests.
var readFile = os.ReadFile
