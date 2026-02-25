package handlers

import (
	"net/http"

	"cortex/internal/logger"
)

type ROSHandler struct {
	log *logger.Logger
}

func NewROSHandler(l *logger.Logger) *ROSHandler {
	return &ROSHandler{log: l}
}

func (h *ROSHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ros/nodes", h.Nodes)
	mux.HandleFunc("GET /ros/topics", h.Topics)
	mux.HandleFunc("GET /ros/topic/{name}/echo", h.Echo)
	mux.HandleFunc("GET /ros/services", h.Services)
	mux.HandleFunc("GET /ros/graph", h.Graph)
}

func (h *ROSHandler) Nodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": []map[string]any{
			{"name": "/navigation", "namespace": "/", "status": "active"},
			{"name": "/lidar_driver", "namespace": "/sensors", "status": "active"},
			{"name": "/camera", "namespace": "/sensors", "status": "active"},
		},
	})
}

func (h *ROSHandler) Topics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"topics": []map[string]any{
			{"name": "/cmd_vel", "type": "geometry_msgs/msg/Twist", "publishers": 1, "subscribers": 2},
			{"name": "/scan", "type": "sensor_msgs/msg/LaserScan", "publishers": 1, "subscribers": 1},
			{"name": "/image_raw", "type": "sensor_msgs/msg/Image", "publishers": 1, "subscribers": 1},
		},
	})
}

func (h *ROSHandler) Echo(w http.ResponseWriter, r *http.Request) {
	// Path param uses underscores in place of leading slashes per the spec.
	name := "/" + r.PathValue("name")
	writeJSON(w, http.StatusOK, map[string]any{
		"topic":    name,
		"type":     "geometry_msgs/msg/Twist",
		"messages": []any{},
	})
}

func (h *ROSHandler) Services(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"services": []map[string]any{
			{"name": "/navigation/reset", "type": "std_srvs/srv/Trigger"},
			{"name": "/navigation/set_goal", "type": "nav2_msgs/srv/NavigateToPose"},
		},
	})
}

func (h *ROSHandler) Graph(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": []string{"/navigation", "/lidar_driver", "/camera"},
		"edges": []map[string]any{
			{"from": "/lidar_driver", "to": "/navigation", "topic": "/scan"},
			{"from": "/camera", "to": "/navigation", "topic": "/image_raw"},
		},
	})
}
