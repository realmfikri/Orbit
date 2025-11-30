package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"orbit/backend/simulation"
)

// Server exposes HTTP and WebSocket endpoints for the truck simulation.
type Server struct {
	sim          *simulation.Manager
	wsUpgrader   websocket.Upgrader
	wsInterval   time.Duration
	wsChunkSize  int
	defaultPage  int
	defaultLimit int
}

// NewServer constructs a Server with sensible defaults for pagination and streaming.
func NewServer(sim *simulation.Manager) *Server {
	return &Server{
		sim: sim,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		wsInterval:   2 * time.Second,
		wsChunkSize:  200,
		defaultPage:  1,
		defaultLimit: 100,
	}
}

// Routes returns an http.Handler that serves all endpoints.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReadiness)
	mux.HandleFunc("/api/trucks", s.handleTrucks)
	mux.HandleFunc("/ws/trucks", s.handleTrucksWebSocket)
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
		log.Printf("websocket upgrade failed: %v", err)
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
		log.Printf("websocket initial send failed: %v", err)
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := sendSnapshot(); err != nil {
				log.Printf("websocket send failed: %v", err)
				return
			}
		}
	}
}
