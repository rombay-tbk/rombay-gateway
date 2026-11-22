package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDGeneratesAndPreserves(t *testing.T) {
	t.Parallel()

	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	h := RequestID(next)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen == "" || rec.Header().Get(headerRequestID) != seen {
		t.Fatalf("generated id header=%q ctx=%q", rec.Header().Get(headerRequestID), seen)
	}
	if strings.Count(seen, "-") != 4 {
		t.Fatalf("id = %s", seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(headerRequestID, "ord-2026.11_21")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen != "ord-2026.11_21" || rec.Header().Get(headerRequestID) != seen {
		t.Fatalf("preserved id header=%q ctx=%q", rec.Header().Get(headerRequestID), seen)
	}
}

func TestRequestIDReplacesMalformed(t *testing.T) {
	t.Parallel()

	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))
	for _, raw := range []string{"bad id", "line\nbreak", strings.Repeat("a", 129), " "} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(headerRequestID, raw)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if seen == raw || seen == "" {
			t.Fatalf("raw %q produced %q", raw, seen)
		}
	}
}

func TestLoggingIncludesRequestID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := RequestID(Logging(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})))

	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.Header.Set(headerRequestID, "req-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log %q: %v", buf.String(), err)
	}
	if line["request_id"] != "req-1" || line["method"] != "POST" || line["path"] != "/payments" || line["status"] != float64(201) {
		t.Fatalf("line = %#v", line)
	}
}

func TestLoggingKeepsSuccessfulProbesAtDebug(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := Logging(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if buf.Len() != 0 {
		t.Fatalf("info log captured probe: %s", buf.String())
	}
}
