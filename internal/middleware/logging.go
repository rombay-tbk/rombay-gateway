package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging writes one structured access line per request.
// Probe successes stay at debug so readiness traffic does not flood info logs.
func Logging(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		level := slog.LevelInfo
		switch {
		case r.URL.Path == "/healthz" || r.URL.Path == "/readyz":
			if sw.status >= http.StatusInternalServerError {
				level = slog.LevelWarn
			} else {
				level = slog.LevelDebug
			}
		case sw.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case sw.status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		logger.Log(r.Context(), level, "request",
			"request_id", RequestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
