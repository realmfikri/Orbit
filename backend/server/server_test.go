package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"orbit/backend/simulation"
)

func newTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	cfg := simulation.Config{
		NumTrucks:      5,
		Seed:           1,
		SpeedMin:       1,
		SpeedMax:       1,
		UpdateInterval: 10 * time.Millisecond,
		StartPoints:    []simulation.Point{{Lat: 0, Lon: 0}},
		EndPoints:      []simulation.Point{{Lat: 0, Lon: 0.01}},
	}
	mgr := simulation.NewManager(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start simulation: %v", err)
	}

	srv := NewServer(mgr)

	cleanup := func() {
		cancel()
		mgr.Stop()
	}
	return srv, cleanup
}

func TestHealthAndReadiness(t *testing.T) {
	srv, cleanup := newTestServer(t)
	defer cleanup()

	router := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "ok" {
		t.Fatalf("health check failed: code %d body %q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "ready" {
		t.Fatalf("readiness check failed: code %d body %q", rr.Code, rr.Body.String())
	}
}

func TestTrucksPagination(t *testing.T) {
	srv, cleanup := newTestServer(t)
	defer cleanup()

	router := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/trucks?page=2&size=2", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	var resp paginatedResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Page != 2 || resp.Size != 2 {
		t.Fatalf("unexpected pagination metadata: %+v", resp)
	}
	if len(resp.Trucks) != 2 {
		t.Fatalf("expected 2 trucks, got %d", len(resp.Trucks))
	}
	if resp.Total != 5 {
		t.Fatalf("expected total 5 got %d", resp.Total)
	}
}

func TestSimulationConfigEndpoint(t *testing.T) {
	srv, cleanup := newTestServer(t)
	defer cleanup()

	router := srv.Routes()

	t.Run("get current config", func(t *testing.T) {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/simulation/config", nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var resp simulationConfigResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if resp.NumTrucks != 5 || resp.UpdateIntervalMs <= 0 {
			t.Fatalf("unexpected config response: %+v", resp)
		}
	})

	t.Run("apply new config", func(t *testing.T) {
		body := strings.NewReader(`{"numTrucks":3,"updateIntervalMs":25,"boundingBox":{"minLat":0,"minLon":0,"maxLat":1,"maxLon":1}}`)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/simulation/config", body))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var resp simulationConfigResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if resp.NumTrucks != 3 || resp.UpdateIntervalMs != 25 || resp.BoundingBox == nil {
			t.Fatalf("unexpected response after update: %+v", resp)
		}

		if got := len(srv.sim.Trucks()); got != 3 {
			t.Fatalf("expected 3 trucks after update, got %d", got)
		}
		if srv.sim.Config().RouteBounds == nil {
			t.Fatalf("expected bounding box applied")
		}
	})

	t.Run("restore defaults", func(t *testing.T) {
		body := strings.NewReader(`{"restoreDefaults":true}`)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/simulation/config", body))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var resp simulationConfigResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if resp.NumTrucks != 5 {
			t.Fatalf("expected defaults restored, got %+v", resp)
		}
	})
}

func TestWebSocketStream(t *testing.T) {
	srv, cleanup := newTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	url := "ws" + ts.URL[len("http"):] + "/ws/trucks"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var trucks []simulation.Truck
	if err := conn.ReadJSON(&trucks); err != nil {
		t.Fatalf("failed to read initial message: %v", err)
	}
	if len(trucks) == 0 {
		t.Fatalf("expected some trucks in websocket message")
	}
}
