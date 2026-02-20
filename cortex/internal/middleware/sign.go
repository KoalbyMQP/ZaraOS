package middleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"cortex/internal/auth"
	"cortex/internal/logger"
)

// RequireAuth verifies X-Timestamp and X-Signature on every request.
// Delegates to the next handler on success; returns 401 on any failure.
func RequireAuth(store *auth.Store, log *logger.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamp := r.Header.Get("X-Timestamp")
		signature := r.Header.Get("X-Signature")

		if timestamp == "" || signature == "" {
			log.Warn("auth rejected — missing headers",
				"method", r.Method,
				"path", r.URL.Path,
				"missing", missingHeaders(timestamp, signature),
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"missing X-Timestamp or X-Signature header"}`))
			return
		}

		// Buffer body so we can both verify the signature and let the handler read it.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"failed to read request body"}`))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		sess, err := store.ValidateRequest(timestamp, r.Method, r.URL.Path, string(body), signature)
		if err != nil {
			log.Warn("auth rejected",
				"method", r.Method,
				"path", r.URL.Path,
				"reason", err.Error(),
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}

		log.Debug("auth ok", "label", sess.Label, "prefix", sess.Prefix, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Logging wraps every request with method, path, status, and latency.
func Logging(log *logger.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		lat := time.Since(start)

		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"latency", lat.Round(time.Microsecond).String(),
		}

		switch {
		case rw.status >= 500:
			log.Error("request", fields...)
		case rw.status >= 400:
			log.Warn("request", fields...)
		default:
			log.Info("request", fields...)
		}
	})
}

func missingHeaders(timestamp, signature string) string {
	switch {
	case timestamp == "" && signature == "":
		return "X-Timestamp, X-Signature"
	case timestamp == "":
		return "X-Timestamp"
	default:
		return "X-Signature"
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
