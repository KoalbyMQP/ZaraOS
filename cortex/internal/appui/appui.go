// Package appui discovers the Driver Station UI an app ships inside its
// container image and keeps it in memory so cortex can serve it.
//
// An app opts in by putting a UI bundle at BundleDir in its image: a
// zara-ui.json descriptor plus the Module Federation build it points to.
// Images without that directory simply have no UI.
package appui

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"cortex/internal/logger"
)

const (
	// BundleDir is the well-known directory inside an app image that holds
	// its UI bundle. It sits next to the embedded package manifest.
	BundleDir = "/etc/zaraos/ui"
	// DescriptorName is the descriptor the Driver Station loads first.
	DescriptorName = "zara-ui.json"

	maxBundleBytes = 64 << 20
	maxBundleFiles = 4096
	extractTimeout = 60 * time.Second
)

// ErrNoBundle means the image has no UI bundle. That is the normal case for
// apps without a UI, not a failure.
var ErrNoBundle = errors.New("image has no UI bundle")

// File is one servable file of a bundle.
type File struct {
	Data []byte
	ETag string
}

// Bundle is the UI of one app version, as extracted from an image.
type Bundle struct {
	App      string
	Version  string
	Image    string
	LoadedAt time.Time
	files    map[string]File
}

// File returns the file at a slash-separated path relative to the bundle root.
func (b *Bundle) File(name string) (File, bool) {
	f, ok := b.files[name]
	return f, ok
}

// ExtractFunc returns the files under BundleDir of an image, keyed by their
// slash-separated path relative to it.
type ExtractFunc func(ctx context.Context, image string) (map[string][]byte, error)

type key struct{ app, version string }

// Store holds the current bundle of every app version cortex has seen running.
//
// Bundles are keyed by app and version rather than by instance so their URLs
// survive a restart: the Driver Station identifies a federation build by its
// address and will not load the same build name from a second one.
type Store struct {
	extract ExtractFunc
	log     *logger.Logger

	mu      sync.RWMutex
	bundles map[key]*Bundle
	latest  map[key]uint64  // newest load started per key
	seen    map[string]bool // instance IDs already examined
}

func NewStore(extract ExtractFunc, log *logger.Logger) *Store {
	return &Store{
		extract: extract,
		log:     log,
		bundles: make(map[key]*Bundle),
		latest:  make(map[key]uint64),
		seen:    make(map[string]bool),
	}
}

// Observe is called whenever an instance is reported. The first time a
// running instance is seen, its image is examined in the background, so every
// new instance (start, restart, update, autostart) refreshes the bundle
// without each of those code paths having to ask for it.
func (s *Store) Observe(instanceID, app, version, image, state string) {
	if state != "running" || image == "" {
		return
	}
	s.mu.Lock()
	if s.seen[instanceID] {
		s.mu.Unlock()
		return
	}
	s.seen[instanceID] = true
	s.mu.Unlock()

	go func() {
		err := s.Load(app, version, image)
		switch {
		case err == nil:
			s.log.Event("APP UI LOADED", "app", app, "version", version, "image", image)
		case errors.Is(err, ErrNoBundle):
			s.log.Debug("app ui: none in image", "app", app, "image", image)
		default:
			s.log.Warn("app ui: could not load", "app", app, "image", image, "err", err)
		}
	}()
}

// Load extracts the bundle from image and makes it current for app/version.
// If the image has no usable bundle, any previous one is dropped, so the UI
// on offer always matches the image that last ran.
func (s *Store) Load(app, version, image string) error {
	k := key{app, version}
	s.mu.Lock()
	s.latest[k]++
	mine := s.latest[k]
	s.mu.Unlock()

	bundle, err := s.build(app, version, image)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest[k] != mine {
		return err // a newer instance's load owns this key now
	}
	if err != nil {
		delete(s.bundles, k)
		return err
	}
	s.bundles[k] = bundle
	return nil
}

func (s *Store) build(app, version, image string) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), extractTimeout)
	defer cancel()
	raw, err := s.extract(ctx, image)
	if err != nil {
		return nil, err
	}
	descriptor, ok := raw[DescriptorName]
	if !ok {
		return nil, fmt.Errorf("%s has no %s", BundleDir, DescriptorName)
	}
	stamped, err := stampDescriptor(descriptor, app, version)
	if err != nil {
		return nil, err
	}
	raw[DescriptorName] = stamped

	files := make(map[string]File, len(raw))
	for name, data := range raw {
		sum := sha256.Sum256(data)
		files[name] = File{Data: data, ETag: `"` + hex.EncodeToString(sum[:16]) + `"`}
	}
	return &Bundle{
		App:      app,
		Version:  version,
		Image:    image,
		LoadedAt: time.Now(),
		files:    files,
	}, nil
}

// Bundle returns the current bundle for an app version.
func (s *Store) Bundle(app, version string) (*Bundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bundles[key{app, version}]
	return b, ok
}

// DescriptorURL returns the path, relative to the robot origin, at which the
// descriptor of an app version is served.
func (s *Store) DescriptorURL(app, version string) (string, bool) {
	if _, ok := s.Bundle(app, version); !ok {
		return "", false
	}
	return "/ui/" + url.PathEscape(app) + "/" + url.PathEscape(version) + "/" + DescriptorName, true
}

// stampDescriptor sets appId and appVersion to the identity cortex runs the
// app under.
//
// The Driver Station refuses a descriptor whose identity differs from the
// instance's. An image cannot know the name and version it will be started
// as (a locally built image runs as version "dev"), so cortex, which does,
// fills them in.
func stampDescriptor(raw []byte, app, version string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return nil, fmt.Errorf("%s is not a JSON object", DescriptorName)
	}
	doc["appId"], _ = json.Marshal(app)
	doc["appVersion"], _ = json.Marshal(version)
	return json.Marshal(doc)
}

// ReadBundle reads a tar stream of a bundle directory into memory.
//
// Only regular files are kept. Links and special files are dropped rather
// than followed, and names are resolved against the bundle root, so nothing
// an image puts in its bundle can make cortex serve a file from outside it.
func ReadBundle(r io.Reader) (map[string][]byte, error) {
	files := make(map[string][]byte)
	total := int64(0)
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read UI bundle: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(path.Clean("/"+hdr.Name), "/")
		if name == "" {
			continue
		}
		if len(files) >= maxBundleFiles {
			return nil, fmt.Errorf("UI bundle has more than %d files", maxBundleFiles)
		}
		total += hdr.Size
		if hdr.Size < 0 || total > maxBundleBytes {
			return nil, fmt.Errorf("UI bundle is larger than %d MiB", maxBundleBytes>>20)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read UI bundle: %w", err)
		}
		files[name] = data
	}
}

// ImageExtractor returns an ExtractFunc that reads BundleDir out of an image
// by running tar in a throwaway container, the same way the embedded package
// manifest is read. cli is called per extraction so detection stays lazy.
func ImageExtractor(cli func() string) ExtractFunc {
	return func(ctx context.Context, image string) (map[string][]byte, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cmd := exec.CommandContext(ctx, cli(), "run", "--rm", "--entrypoint", "",
			image, "tar", "-cf", "-", "-C", BundleDir, ".")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}

		files, readErr := ReadBundle(stdout)
		if readErr != nil {
			cancel() // stop tar; nothing will read the rest of its output
		}
		waitErr := cmd.Wait()

		msg := strings.TrimSpace(stderr.String())
		switch {
		case waitErr != nil && strings.Contains(msg, "No such file or directory"):
			return nil, ErrNoBundle
		case readErr != nil:
			return nil, readErr
		case waitErr != nil:
			return nil, fmt.Errorf("read %s from %s: %s: %w", BundleDir, image, msg, waitErr)
		}
		return files, nil
	}
}
