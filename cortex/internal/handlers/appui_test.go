package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cortex/internal/appui"
	"cortex/internal/logger"
	"cortex/internal/store"
)

// newAppUITestServer builds the routes this feature adds, with one running
// instance of "demo" whose image ships a UI bundle.
func newAppUITestServer(t *testing.T) (*httptest.Server, *store.Instance) {
	t.Helper()
	log := logger.New()
	descriptor := `{"schemaVersion":1,"pages":{"overview":{"title":"Overview","module":"./Overview"}}}`
	ui := appui.NewStore(func(context.Context, string) (map[string][]byte, error) {
		return map[string][]byte{
			appui.DescriptorName:  []byte(descriptor),
			"remoteEntry.js":      []byte("export const ok = true;"),
			"assets/overview.css": []byte("h1{color:red}"),
		}, nil
	}, log)

	st := store.New()
	inst := st.CreateInstance("demo", "dev", "demo:dev")
	st.SetInstanceState(inst.ID, "running", nil)
	if err := ui.Load(inst.App, inst.Version, inst.Image); err != nil {
		t.Fatalf("Load: %v", err)
	}

	instances := NewInstancesHandler(st, nil, log)
	instances.SetAppUI(ui)
	mux := http.NewServeMux()
	NewAppUIHandler(ui, log).RegisterPublic(mux)
	mux.HandleFunc("GET /instances/{id}/summary", func(w http.ResponseWriter, r *http.Request) {
		got, _ := st.GetInstance(r.PathValue("id"))
		writeJSON(w, http.StatusOK, instances.withUI(got, instanceSummary(got)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, inst
}

func get(t *testing.T, url string, header ...string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// The descriptor_url an instance advertises must be fetchable as given and
// return a descriptor the Driver Station will accept for that instance.
func TestInstanceAdvertisesAFetchableDescriptor(t *testing.T) {
	srv, inst := newAppUITestServer(t)

	_, body := get(t, srv.URL+"/instances/"+inst.ID+"/summary")
	var summary struct {
		UI struct {
			DescriptorURL string `json:"descriptor_url"`
		} `json:"ui"`
	}
	if err := json.Unmarshal([]byte(body), &summary); err != nil || summary.UI.DescriptorURL == "" {
		t.Fatalf("instance has no ui.descriptor_url: %s", body)
	}

	resp, body := get(t, srv.URL+summary.UI.DescriptorURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("descriptor status = %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("descriptor Content-Type = %q", ct)
	}
	var descriptor map[string]any
	if err := json.Unmarshal([]byte(body), &descriptor); err != nil {
		t.Fatalf("descriptor is not JSON: %v", err)
	}
	if descriptor["appId"] != inst.App || descriptor["appVersion"] != inst.Version {
		t.Errorf("descriptor identity = %v/%v, want %s/%s", descriptor["appId"], descriptor["appVersion"], inst.App, inst.Version)
	}
}

func TestAssetsAreServedWithScriptTypesAndRevalidation(t *testing.T) {
	srv, _ := newAppUITestServer(t)

	resp, body := get(t, srv.URL+"/ui/demo/dev/remoteEntry.js")
	if resp.StatusCode != http.StatusOK || body != "export const ok = true;" {
		t.Fatalf("remoteEntry.js = %d %q", resp.StatusCode, body)
	}
	// Browsers refuse to run a module script that is not served as JavaScript.
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("remoteEntry.js Content-Type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if resp, _ := get(t, srv.URL+"/ui/demo/dev/remoteEntry.js", "If-None-Match", etag); resp.StatusCode != http.StatusNotModified {
		t.Errorf("revalidation status = %d, want 304", resp.StatusCode)
	}

	if resp, body := get(t, srv.URL+"/ui/demo/dev/assets/overview.css"); resp.StatusCode != http.StatusOK ||
		body != "h1{color:red}" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css") {
		t.Errorf("nested asset = %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	for _, path := range []string{"/ui/demo/dev/missing.js", "/ui/demo/dev/", "/ui/demo/v2/remoteEntry.js", "/ui/other/dev/remoteEntry.js"} {
		if resp, _ := get(t, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}
