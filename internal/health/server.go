package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server provides HTTP health check endpoints for Cloud Run.
type Server struct {
	pool             *pgxpool.Pool
	port             int
	highFreqLastTick atomic.Value // time.Time
	lowFreqLastTick  atomic.Value // time.Time
	srv              *http.Server
}

// NewServer creates a new health check HTTP server.
func NewServer(pool *pgxpool.Pool, port int) *Server {
	s := &Server{
		pool: pool,
		port: port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

// SetHighFreqTick records the latest high-freq tick time.
func (s *Server) SetHighFreqTick(t time.Time) {
	s.highFreqLastTick.Store(t)
}

// SetLowFreqTick records the latest low-freq tick time.
func (s *Server) SetLowFreqTick(t time.Time) {
	s.lowFreqLastTick.Store(t)
}

// Start begins serving. Blocks until context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("health server starting", "port", s.port)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
	}()

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.pool.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "db not ready: %v", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ready")
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"status": "running",
	}

	if v := s.highFreqLastTick.Load(); v != nil {
		metrics["high_freq_last_tick"] = v.(time.Time).Format(time.RFC3339)
	}
	if v := s.lowFreqLastTick.Load(); v != nil {
		metrics["low_freq_last_tick"] = v.(time.Time).Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
