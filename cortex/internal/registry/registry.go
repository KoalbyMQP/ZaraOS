package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultTTL = 5 * time.Minute

type Version struct {
	Version     string
	PublishedAt time.Time
	Changelog   string
}

type App struct {
	Name     string
	Repo     string
	Versions []Version
}

type repoCache struct {
	apps      []App
	fetchedAt time.Time
}

type Registry struct {
	githubOrg string
	dockerOrg string
	ttl       time.Duration
	repos     []string
	mu        sync.RWMutex
	cache     map[string]*repoCache
	client    *http.Client
}

func New() *Registry {
	ttl := defaultTTL
	if v := os.Getenv("REGISTRY_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}

	var repos []string
	if v := os.Getenv("GITHUB_REPOS"); v != "" {
		for _, r := range strings.Split(v, ",") {
			if r = strings.TrimSpace(r); r != "" {
				repos = append(repos, r)
			}
		}
	}

	return &Registry{
		githubOrg: envOr("GITHUB_ORG", "KoalbyMQP"),
		dockerOrg: envOr("DOCKERHUB_ORG", "koalbymqp"),
		ttl:       ttl,
		repos:     repos,
		cache:     make(map[string]*repoCache),
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ListApps returns all discovered apps across all watched repos.
func (r *Registry) ListApps() []App {
	var all []App
	for _, repo := range r.repos {
		all = append(all, r.appsForRepo(repo)...)
	}
	return all
}

// GetApp finds an app by name across all repos.
func (r *Registry) GetApp(name string) (App, bool) {
	for _, app := range r.ListApps() {
		if app.Name == name {
			return app, true
		}
	}
	return App{}, false
}

// LatestVersion returns the most recently published version tag for an app.
func (r *Registry) LatestVersion(appName string) string {
	app, ok := r.GetApp(appName)
	if !ok || len(app.Versions) == 0 {
		return ""
	}
	return app.Versions[0].Version
}

// ResolveVersion returns the concrete version string, handling "latest".
func (r *Registry) ResolveVersion(appName, version string) (string, bool) {
	app, ok := r.GetApp(appName)
	if !ok {
		return "", false
	}
	if version == "latest" {
		if len(app.Versions) == 0 {
			return "", false
		}
		return app.Versions[0].Version, true
	}
	for _, v := range app.Versions {
		if v.Version == version {
			return version, true
		}
	}
	return "", false
}

// ImageRef returns the full Docker Hub image reference for a package and tag.
func (r *Registry) ImageRef(pkg, tag string) string {
	return fmt.Sprintf("docker.io/%s/%s:%s", r.dockerOrg, pkg, tag)
}

func (r *Registry) appsForRepo(repo string) []App {
	r.mu.RLock()
	c, ok := r.cache[repo]
	r.mu.RUnlock()

	if ok && time.Since(c.fetchedAt) < r.ttl {
		return c.apps
	}

	apps, err := r.fetchRepo(repo)
	if err != nil {
		// Return stale on error rather than nothing.
		if ok {
			return c.apps
		}
		return nil
	}

	r.mu.Lock()
	r.cache[repo] = &repoCache{apps: apps, fetchedAt: time.Now()}
	r.mu.Unlock()
	return apps
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

func (r *Registry) fetchRepo(repo string) ([]App, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", r.githubOrg, repo)
	resp, err := r.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API %d for %s", resp.StatusCode, repo)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode %s: %w", repo, err)
	}

	byPkg := make(map[string][]Version)
	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		pkg := packageNameFromRelease(rel.Name, rel.TagName)
		if pkg == "" {
			continue
		}
		byPkg[pkg] = append(byPkg[pkg], Version{
			Version:     rel.TagName,
			PublishedAt: rel.PublishedAt,
			Changelog:   strings.TrimSpace(rel.Body),
		})
	}

	apps := make([]App, 0, len(byPkg))
	for pkg, versions := range byPkg {
		// Newest first.
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].PublishedAt.After(versions[j].PublishedAt)
		})
		apps = append(apps, App{
			Name:     pkg,
			Repo:     repo,
			Versions: versions,
		})
	}
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})
	return apps, nil
}

// packageNameFromRelease derives a slug from the release title by stripping the
// version tag, then lowercasing and replacing spaces/underscores with hyphens.
//
// "Python Example v0.0.1" + tag "v0.0.1" → "python-example"
func packageNameFromRelease(name, tag string) string {
	// Strip trailing tag from name (mirrors getReleaseGroupName in driver-station).
	n := name
	if tag != "" && strings.HasSuffix(n, tag) {
		n = strings.TrimSuffix(n, tag)
	}
	n = strings.TrimRight(n, " -_")
	n = strings.TrimSpace(n)
	if n == "" {
		return ""
	}
	// Slugify: lowercase, spaces/underscores → hyphens, collapse multiple hyphens.
	n = strings.ToLower(n)
	n = strings.NewReplacer(" ", "-", "_", "-").Replace(n)
	for strings.Contains(n, "--") {
		n = strings.ReplaceAll(n, "--", "-")
	}
	return n
}
