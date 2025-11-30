package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"orbit/backend/simulation"
)

var apiLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "orbit_api_latency_seconds",
	Help:    "Time spent serving HTTP handlers.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "path", "status"})

func init() {
	prometheus.MustRegister(apiLatency)
}

// Server exposes HTTP and WebSocket endpoints for the truck simulation.
type Server struct {
	sim               *simulation.Manager
	wsUpgrader        websocket.Upgrader
	wsInterval        time.Duration
	wsChunkSize       int
	defaultPage       int
	defaultLimit      int
	logger            *slog.Logger
	correlationHeader string
	adminEnabled      bool
}

// NewServer constructs a Server with sensible defaults for pagination and streaming.
func NewServer(sim *simulation.Manager) *Server {
	return &Server{
		sim: sim,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		wsInterval:        2 * time.Second,
		wsChunkSize:       200,
		defaultPage:       1,
		defaultLimit:      100,
		logger:            slog.Default(),
		correlationHeader: "X-Correlation-ID",
	}
}

// WithAdminEnabled enables admin-only endpoints like pprof.
func (s *Server) WithAdminEnabled() *Server {
	s.adminEnabled = true
	return s
}

// WithLogger configures structured logging.
func (s *Server) WithLogger(logger *slog.Logger) *Server {
	if logger != nil {
		s.logger = logger
	}
	return s
}

// WithCorrelationHeader configures the header used to propagate correlation IDs.
func (s *Server) WithCorrelationHeader(header string) *Server {
	if header != "" {
		s.correlationHeader = header
	}
	return s
}

// Routes returns an http.Handler that serves all endpoints.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.wrap(s.handleHealth))
	mux.HandleFunc("/readyz", s.wrap(s.handleReadiness))
	mux.HandleFunc("/api/trucks", s.wrap(s.handleTrucks))
	mux.HandleFunc("/ws/trucks", s.wrap(s.handleTrucksWebSocket))
	mux.Handle("/metrics", promhttp.Handler())

	if s.adminEnabled {
		mux.HandleFunc("/admin/debug/pprof/", pprof.Index)
		mux.HandleFunc("/admin/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/admin/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/admin/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/admin/debug/pprof/trace", pprof.Trace)
	}
	return mux
}

type paginatedResponse struct {
	Trucks []simulation.Truck `json:"trucks"`
	Page   int                `json:"page"`
	Size   int                `json:"size"`
	Total  int                `json:"total"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if s.sim == nil || !s.sim.Started() {
		http.Error(w, "simulation not started", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) handleTrucks(w http.ResponseWriter, r *http.Request) {
	page := s.defaultPage
	size := s.defaultLimit

	if v := r.URL.Query().Get("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if v := r.URL.Query().Get("size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			size = parsed
		}
	}

	snapshot := s.sim.Trucks()
	total := len(snapshot)

	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}

	resp := paginatedResponse{
		Trucks: snapshot[start:end],
		Page:   page,
		Size:   size,
		Total:  total,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleTrucksWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "err", err, "correlation_id", correlationIDFromContext(r.Context()))
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(s.wsInterval)
	defer ticker.Stop()

	sendSnapshot := func() error {
		trucks := s.sim.Trucks()
		if s.wsChunkSize > 0 && len(trucks) > s.wsChunkSize {
			trucks = trucks[:s.wsChunkSize]
		}
		return conn.WriteJSON(trucks)
	}

	if err := sendSnapshot(); err != nil {
		s.logger.Error("websocket initial send failed", "err", err, "correlation_id", correlationIDFromContext(r.Context()))
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := sendSnapshot(); err != nil {
				s.logger.Error("websocket send failed", "err", err, "correlation_id", correlationIDFromContext(r.Context()))
				return
			}
		}
	}
}
