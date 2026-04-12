package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cortex/internal/logger"
	"cortex/internal/store"
)

type ImagesHandler struct {
	store *store.Store
	log   *logger.Logger
}

func NewImagesHandler(s *store.Store, l *logger.Logger) *ImagesHandler {
	return &ImagesHandler{store: s, log: l}
}

func (h *ImagesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /images", h.List)
	mux.HandleFunc("GET /images/{name}/tags", h.Tags)
	mux.HandleFunc("DELETE /images/{name}", h.Delete)
}

// nerdctlImage represents the JSON output of nerdctl images --format '{{json .}}'.
type nerdctlImage struct {
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	ID         string `json:"ID"`
	Size       string `json:"Size"`
	CreatedAt  string `json:"CreatedAt"`
}

// localImage is the aggregated view: one entry per unique repository.
type localImage struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
	ID         string   `json:"id"`
	Size       string   `json:"size"`
	CreatedAt  string   `json:"created_at"`
}

// List returns all locally available images grouped by repository.
func (h *ImagesHandler) List(w http.ResponseWriter, r *http.Request) {
	raw, err := containerExec("images", "--format", "{{json .}}")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list images: "+err.Error())
		return
	}

	images := parseNerdctlImages(raw)

	// Aggregate by repository.
	grouped := aggregateImages(images)

	writeJSON(w, http.StatusOK, map[string]any{"images": grouped})
}

// Tags returns all tags for a given image repository.
// The {name} path value uses underscores in place of slashes for nested names,
// e.g. "koalbymqp_ros2-nav" → "koalbymqp/ros2-nav".
func (h *ImagesHandler) Tags(w http.ResponseWriter, r *http.Request) {
	name := pathToRepo(r.PathValue("name"))

	raw, err := containerExec("images", "--format", "{{json .}}")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list images: "+err.Error())
		return
	}

	images := parseNerdctlImages(raw)

	var tags []string
	for _, img := range images {
		if img.Repository == name && img.Tag != "<none>" {
			tags = append(tags, img.Tag)
		}
	}

	if len(tags) == 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no tags found for image %s", name))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repository": name,
		"tags":       tags,
	})
}

// Delete removes a local image. Accepts an optional ?tag= query parameter.
// Without a tag, the entire repository (all tags) is removed.
// With ?tag=<tag>, only that specific tag is removed.
// Pass ?force=true to force-remove even if containers reference the image.
func (h *ImagesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := pathToRepo(r.PathValue("name"))
	tag := r.URL.Query().Get("tag")
	force := r.URL.Query().Get("force") == "true"

	var ref string
	if tag != "" {
		ref = name + ":" + tag
	} else {
		ref = name
	}

	args := []string{"rmi"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, ref)

	out, err := containerExec(args...)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "No such image") || strings.Contains(msg, "not found") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("image %s not found", ref))
			return
		}
		writeError(w, http.StatusConflict, "failed to remove image: "+msg)
		return
	}

	h.log.Event("IMAGE DELETED", "ref", ref, "force", force)
	h.store.AddEvent("image_deleted", map[string]any{
		"ref":   ref,
		"force": force,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": ref,
		"detail":  strings.TrimSpace(out),
	})
}

// --- helpers ---

// pathToRepo converts a URL-safe name back to a repository reference.
// Underscores are treated as path separators: "docker.io_koalbymqp_ros2-nav" → "docker.io/koalbymqp/ros2-nav"
func pathToRepo(name string) string {
	return strings.ReplaceAll(name, "_", "/")
}

// parseNerdctlImages parses the newline-delimited JSON output from nerdctl images.
func parseNerdctlImages(raw string) []nerdctlImage {
	var images []nerdctlImage
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var img nerdctlImage
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}
		if img.Repository == "" || img.Repository == "<none>" {
			continue
		}
		images = append(images, img)
	}
	return images
}

// aggregateImages groups images by repository and collects their tags.
func aggregateImages(images []nerdctlImage) []localImage {
	type entry struct {
		tags      []string
		id        string
		size      string
		createdAt string
	}

	seen := make(map[string]*entry)
	var order []string

	for _, img := range images {
		e, ok := seen[img.Repository]
		if !ok {
			e = &entry{
				id:        img.ID,
				size:      img.Size,
				createdAt: img.CreatedAt,
			}
			seen[img.Repository] = e
			order = append(order, img.Repository)
		}
		if img.Tag != "<none>" {
			e.tags = append(e.tags, img.Tag)
		}
	}

	out := make([]localImage, 0, len(order))
	for _, repo := range order {
		e := seen[repo]
		tags := e.tags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, localImage{
			Repository: repo,
			Tags:       tags,
			ID:         e.id,
			Size:       e.size,
			CreatedAt:  e.createdAt,
		})
	}
	return out
}
