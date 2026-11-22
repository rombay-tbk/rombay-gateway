package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rombay-tbk/rombay-gateway/internal/health"
	"github.com/rombay-tbk/rombay-gateway/internal/middleware"
)

// Dependency is a readiness check that can be closed on shutdown.
type Dependency interface {
	health.Checker
	io.Closer
}

// Options wires the process. Dependencies are injected by main.
type Options struct {
	ListenAddr        string
	Logger            *slog.Logger
	ShutdownTimeout   time.Duration
	DependencyTimeout time.Duration
	Dependencies      []Dependency
}

// Runner is a started HTTP server.
type Runner struct {
	srv         *http.Server
	ln          net.Listener
	logger      *slog.Logger
	shutdownFor time.Duration
	deps        []Dependency
	serveErr    chan error
	closeOnce   sync.Once
	closeErr    error
}

// Start listens and serves liveness and readiness. It returns after the socket is open.
func Start(opts Options) (*Runner, error) {
	if opts.ListenAddr == "" {
		return nil, fmt.Errorf("app: listen address is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 15 * time.Second
	}
	if opts.DependencyTimeout <= 0 {
		opts.DependencyTimeout = 2 * time.Second
	}

	checkers := make([]health.Checker, len(opts.Dependencies))
	for i, dep := range opts.Dependencies {
		checkers[i] = dep
	}
	mux := http.NewServeMux()
	health.NewHandler(opts.DependencyTimeout, checkers...).Register(mux)

	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("app: listen: %w", err)
	}

	runner := &Runner{
		ln:          ln,
		logger:      opts.Logger,
		shutdownFor: opts.ShutdownTimeout,
		deps:        opts.Dependencies,
		serveErr:    make(chan error, 1),
		srv: &http.Server{
			Handler:           middleware.RequestID(middleware.Logging(opts.Logger, mux)),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ErrorLog:          slog.NewLogLogger(opts.Logger.Handler(), slog.LevelError),
		},
	}
	go func() {
		err := runner.srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runner.serveErr <- err
		}
		close(runner.serveErr)
	}()
	return runner, nil
}

// Addr is the bound host and port.
func (r *Runner) Addr() string {
	return r.ln.Addr().String()
}

// Wait blocks until ctx is cancelled or the server fails, then drains and closes dependencies.
func (r *Runner) Wait(ctx context.Context) error {
	select {
	case err := <-r.serveErr:
		r.closeDependencies()
		if err == nil {
			err = errors.New("app: server stopped")
		}
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), r.shutdownFor)
		defer cancel()
		err := r.srv.Shutdown(shutCtx)
		r.closeDependencies()
		if err != nil {
			return fmt.Errorf("app: shutdown: %w", err)
		}
		r.logger.Info("shutdown complete")
		return nil
	}
}

func (r *Runner) closeDependencies() {
	r.closeOnce.Do(func() {
		var err error
		for _, dep := range r.deps {
			if closeErr := dep.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close %s: %w", dep.Name(), closeErr))
			}
		}
		r.closeErr = err
		if err != nil {
			r.logger.Error("dependency close failed", "error", err)
		}
	})
}
