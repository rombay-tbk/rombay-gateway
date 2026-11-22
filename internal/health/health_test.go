package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type stubChecker struct {
	name  string
	err   error
	delay time.Duration
	hits  int
	mu    sync.Mutex
}

func (s *stubChecker) Name() string { return s.name }

func (s *stubChecker) Check(ctx context.Context) error {
	s.mu.Lock()
	s.hits++
	s.mu.Unlock()
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.err
}

func TestLiveDoesNotCallDependencies(t *testing.T) {
	t.Parallel()

	redis := &stubChecker{name: "redis", err: errors.New("down")}
	kafka := &stubChecker{name: "kafka", err: errors.New("down")}
	h := NewHandler(time.Second, redis, kafka)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Live(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload body
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "UP" || payload.Checks != nil {
		t.Fatalf("payload = %#v", payload)
	}
	if redis.hits != 0 || kafka.hits != 0 {
		t.Fatalf("liveness called dependencies redis=%d kafka=%d", redis.hits, kafka.hits)
	}
}

func TestReadyUpAndDown(t *testing.T) {
	t.Parallel()

	t.Run("up", func(t *testing.T) {
		t.Parallel()
		h := NewHandler(time.Second, &stubChecker{name: "redis"}, &stubChecker{name: "kafka"})
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		h.Ready(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("down", func(t *testing.T) {
		t.Parallel()
		h := NewHandler(time.Second, &stubChecker{name: "redis"}, &stubChecker{name: "kafka", err: errors.New("metadata failed")})
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		h.Ready(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d", rec.Code)
		}
		var payload body
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Status != "DOWN" || payload.Checks["redis"].Status != "UP" || payload.Checks["kafka"].Status != "DOWN" {
			t.Fatalf("payload = %#v", payload)
		}
		if payload.Checks["kafka"].Error == "" {
			t.Fatal("missing kafka error")
		}
	})
}

func TestReadyTimeout(t *testing.T) {
	t.Parallel()

	h := NewHandler(20*time.Millisecond, &stubChecker{name: "redis", delay: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	h.Ready(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRegisterRoutes(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	NewHandler(time.Second, &stubChecker{name: "redis"}, &stubChecker{name: "kafka"}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/healthz", "/readyz"} {
		res, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, res.StatusCode)
		}
	}
}
