package appui

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"cortex/internal/logger"
)

func staticExtract(files map[string]string, err error) ExtractFunc {
	return func(context.Context, string) (map[string][]byte, error) {
		if err != nil {
			return nil, err
		}
		out := make(map[string][]byte, len(files))
		for name, data := range files {
			out[name] = []byte(data)
		}
		return out, nil
	}
}

// The descriptor an image ships cannot know what cortex will run it as, so
// the served one must carry the instance's identity whatever the image says.
func TestLoadStampsDescriptorWithInstanceIdentity(t *testing.T) {
	s := NewStore(staticExtract(map[string]string{
		DescriptorName:   `{"schemaVersion":1,"appId":"other","appVersion":"9.9.9","pages":{}}`,
		"remoteEntry.js": "export {}",
	}, nil), logger.New())

	if err := s.Load("motor-control", "dev", "motor-control:dev"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, ok := s.Bundle("motor-control", "dev")
	if !ok {
		t.Fatal("no bundle after Load")
	}
	f, _ := b.File(DescriptorName)
	var got map[string]any
	if err := json.Unmarshal(f.Data, &got); err != nil {
		t.Fatalf("served descriptor is not JSON: %v", err)
	}
	if got["appId"] != "motor-control" || got["appVersion"] != "dev" {
		t.Errorf("identity = %v/%v, want motor-control/dev", got["appId"], got["appVersion"])
	}
	if got["schemaVersion"] != float64(1) || got["pages"] == nil {
		t.Errorf("other descriptor fields were not preserved: %v", got)
	}
	if u, _ := s.DescriptorURL("motor-control", "dev"); u != "/ui/motor-control/dev/zara-ui.json" {
		t.Errorf("DescriptorURL = %q", u)
	}
	if _, ok := b.File("remoteEntry.js"); !ok {
		t.Error("bundle is missing remoteEntry.js")
	}
}

func TestDescriptorURLEscapesAppAndVersion(t *testing.T) {
	s := NewStore(staticExtract(map[string]string{DescriptorName: `{}`}, nil), logger.New())
	if err := s.Load("my app", "1.0+build/2", "img"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if u, _ := s.DescriptorURL("my app", "1.0+build/2"); u != "/ui/my%20app/1.0+build%2F2/zara-ui.json" {
		t.Errorf("DescriptorURL = %q", u)
	}
}

func TestLoadRejectsUnusableDescriptors(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"missing descriptor": {"remoteEntry.js": "export {}"},
		"not an object":      {DescriptorName: `[]`},
		"null":               {DescriptorName: `null`},
	} {
		s := NewStore(staticExtract(files, nil), logger.New())
		if err := s.Load("app", "v1", "img"); err == nil {
			t.Errorf("%s: Load succeeded, want an error", name)
		}
		if _, ok := s.Bundle("app", "v1"); ok {
			t.Errorf("%s: bundle is served despite the error", name)
		}
	}
}

// A rebuilt image that no longer ships a UI must not keep serving the old one.
func TestLoadFailureDropsPreviousBundle(t *testing.T) {
	var files map[string]string
	var extractErr error
	s := NewStore(func(ctx context.Context, image string) (map[string][]byte, error) {
		return staticExtract(files, extractErr)(ctx, image)
	}, logger.New())

	files = map[string]string{DescriptorName: `{}`}
	if err := s.Load("app", "dev", "img"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	extractErr = ErrNoBundle
	if err := s.Load("app", "dev", "img"); !errors.Is(err, ErrNoBundle) {
		t.Fatalf("second Load = %v, want ErrNoBundle", err)
	}
	if _, ok := s.DescriptorURL("app", "dev"); ok {
		t.Error("descriptor is still advertised after the image lost its UI")
	}
}

// A slow load for an older instance must not overwrite what a newer instance
// of the same app version already loaded.
func TestOlderLoadDoesNotOverwriteNewer(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	s := NewStore(func(_ context.Context, image string) (map[string][]byte, error) {
		if image == "old" {
			close(started)
			<-release
		}
		return map[string][]byte{DescriptorName: []byte(`{}`), "marker": []byte(image)}, nil
	}, logger.New())

	done := make(chan error)
	go func() { done <- s.Load("app", "dev", "old") }()
	<-started
	if err := s.Load("app", "dev", "new"); err != nil {
		t.Fatalf("Load(new): %v", err)
	}
	close(release)
	<-done

	b, _ := s.Bundle("app", "dev")
	if f, _ := b.File("marker"); string(f.Data) != "new" {
		t.Errorf("current bundle is from %q, want new", f.Data)
	}
}

func TestObserveOnlyExaminesRunningInstancesOnce(t *testing.T) {
	calls := make(chan string, 8)
	s := NewStore(func(_ context.Context, image string) (map[string][]byte, error) {
		calls <- image
		return nil, ErrNoBundle
	}, logger.New())

	s.Observe("a-1", "a", "v1", "img-a", "starting")
	s.Observe("a-1", "a", "v1", "img-a", "stopped")
	select {
	case image := <-calls:
		t.Fatalf("examined %q for an instance that is not running", image)
	default:
	}

	s.Observe("a-1", "a", "v1", "img-a", "running")
	if image := <-calls; image != "img-a" {
		t.Fatalf("examined %q, want img-a", image)
	}
	s.Observe("a-1", "a", "v1", "img-a", "running")
	s.Observe("a-2", "a", "v1", "img-a", "running")
	if image := <-calls; image != "img-a" {
		t.Fatalf("examined %q, want img-a", image)
	}
	select {
	case <-calls:
		t.Fatal("an instance was examined twice")
	default:
	}
}

func TestReadBundleKeepsOnlyRegularFilesInsideTheRoot(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	add := func(hdr tar.Header, body string) {
		hdr.Size = int64(len(body))
		hdr.Mode = 0o644
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add(tar.Header{Name: "./", Typeflag: tar.TypeDir}, "")
	add(tar.Header{Name: "./zara-ui.json", Typeflag: tar.TypeReg}, "{}")
	add(tar.Header{Name: "./assets/", Typeflag: tar.TypeDir}, "")
	add(tar.Header{Name: "./assets/page.js", Typeflag: tar.TypeReg}, "js")
	add(tar.Header{Name: "./shadow", Typeflag: tar.TypeSymlink, Linkname: "/etc/shadow"}, "")
	add(tar.Header{Name: "./hard", Typeflag: tar.TypeLink, Linkname: "zara-ui.json"}, "")
	add(tar.Header{Name: "../../escape.js", Typeflag: tar.TypeReg}, "x")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := ReadBundle(&buf)
	if err != nil {
		t.Fatalf("ReadBundle: %v", err)
	}
	want := map[string]string{"zara-ui.json": "{}", "assets/page.js": "js", "escape.js": "x"}
	if len(files) != len(want) {
		t.Errorf("got files %v, want exactly %v", keys(files), want)
	}
	for name, body := range want {
		if string(files[name]) != body {
			t.Errorf("files[%q] = %q, want %q", name, files[name], body)
		}
	}
}

func TestReadBundleRejectsOversizedBundles(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	// The header alone claims more than the limit; the body never has to exist.
	if err := tw.WriteHeader(&tar.Header{Name: "big.js", Typeflag: tar.TypeReg, Size: maxBundleBytes + 1, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBundle(bytes.NewReader(buf.Bytes())); err == nil {
		t.Error("ReadBundle accepted a bundle over the size limit")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
