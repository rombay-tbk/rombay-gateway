package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker reports whether one dependency can serve traffic.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// Handler serves liveness and readiness.
type Handler struct {
	checkers []Checker
	timeout  time.Duration
}

// NewHandler builds the health endpoints. timeout bounds each readiness call.
func NewHandler(timeout time.Duration, checkers ...Checker) *Handler {
	return &Handler{checkers: checkers, timeout: timeout}
}

// Register mounts GET /healthz and GET /readyz.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.Live)
	mux.HandleFunc("GET /readyz", h.Ready)
}

// Live reports that the process is running. It does not call Redis or Kafka.
func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, body{Status: "UP"})
}

// Ready reports UP only when every dependency check succeeds.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	results := checkAll(ctx, h.checkers)
	status := http.StatusOK
	payload := body{Status: "UP", Checks: make(map[string]checkBody, len(results))}
	for _, result := range results {
		item := checkBody{Status: "UP"}
		if result.err != nil {
			status = http.StatusServiceUnavailable
			payload.Status = "DOWN"
			item.Status = "DOWN"
			item.Error = result.err.Error()
		}
		payload.Checks[result.name] = item
	}
	writeJSON(w, status, payload)
}

type result struct {
	name string
	err  error
}

func checkAll(ctx context.Context, checkers []Checker) []result {
	results := make([]result, len(checkers))
	var wg sync.WaitGroup
	for i, checker := range checkers {
		wg.Add(1)
		go func(i int, checker Checker) {
			defer wg.Done()
			results[i] = result{name: checker.Name(), err: checker.Check(ctx)}
		}(i, checker)
	}
	wg.Wait()
	return results
}

type body struct {
	Status string               `json:"status"`
	Checks map[string]checkBody `json:"checks,omitempty"`
}

type checkBody struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload body) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
