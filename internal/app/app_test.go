package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"
)

type memDep struct {
	name    string
	err     error
	closed  bool
	closeMu sync.Mutex
}

func (d *memDep) Name() string { return d.name }

func (d *memDep) Check(context.Context) error { return d.err }

func (d *memDep) Close() error {
	d.closeMu.Lock()
	d.closed = true
	d.closeMu.Unlock()
	return nil
}

func TestStartHealthAndShutdown(t *testing.T) {
	t.Parallel()

	redis := &memDep{name: "redis"}
	kafka := &memDep{name: "kafka"}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	runner, err := Start(Options{
		ListenAddr:        "127.0.0.1:0",
		Logger:            logger,
		ShutdownTimeout:   time.Second,
		DependencyTimeout: time.Second,
		Dependencies:      []Dependency{redis, kafka},
	})
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: time.Second}
	base := "http://" + runner.Addr()
	live, err := client.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Body.Close()
	if live.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", live.StatusCode)
	}
	if live.Header.Get("X-Request-Id") == "" {
		t.Fatal("missing request id")
	}

	ready, err := client.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	defer ready.Body.Close()
	if ready.StatusCode != http.StatusOK {
		t.Fatalf("readyz = %d", ready.StatusCode)
	}
	var payload struct {
		Status string                     `json:"status"`
		Checks map[string]map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(ready.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "UP" || payload.Checks["redis"]["status"] != "UP" || payload.Checks["kafka"]["status"] != "UP" {
		t.Fatalf("payload = %#v", payload)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Wait(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
	if !redis.closed || !kafka.closed {
		t.Fatalf("closed redis=%v kafka=%v", redis.closed, kafka.closed)
	}
}

func TestReadyFailsClosed(t *testing.T) {
	t.Parallel()

	runner, err := Start(Options{
		ListenAddr:        "127.0.0.1:0",
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		DependencyTimeout: 200 * time.Millisecond,
		Dependencies: []Dependency{
			&memDep{name: "redis"},
			&memDep{name: "kafka", err: errors.New("broker unreachable")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = runner.Wait(ctx)
	})

	res, err := http.Get("http://" + runner.Addr() + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", res.StatusCode, body)
	}
	live, err := http.Get("http://" + runner.Addr() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	live.Body.Close()
	if live.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d while dependency is down", live.StatusCode)
	}
}

func TestStartRequiresAddress(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	_, err := Start(Options{Logger: slog.New(slog.NewTextHandler(&buf, nil))})
	if err == nil {
		t.Fatal("expected error")
	}
}
