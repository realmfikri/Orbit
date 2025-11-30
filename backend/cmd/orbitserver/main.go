package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"orbit/backend/server"
	"orbit/backend/simulation"
)

func main() {
	var (
		addrDefault        = envString("ORBIT_ADDR", ":8080")
		trucksDefault      = envInt("ORBIT_TRUCKS", 2000)
		tickRateDefault    = envDuration("ORBIT_TICK_RATE", time.Second)
		boundingBoxDefault = os.Getenv("ORBIT_BOUNDING_BOX")
		addr               = flag.String("addr", addrDefault, "HTTP listen address")
		enableAdmin        = flag.Bool("enable-admin", false, "enable admin endpoints like pprof")
		trucks             = flag.Int("trucks", trucksDefault, "number of trucks to simulate")
		updateInterval     = flag.Duration("update-interval", tickRateDefault, "simulation update interval")
		tickRate           = flag.String("tick-rate", "", "alias for update-interval; overrides when set")
		boundingBox        = flag.String("bounding-box", boundingBoxDefault, "optional bounding box expressed as minLat,minLon,maxLat,maxLon")
	)
	flag.Parse()

	interval := *updateInterval
	if *tickRate != "" {
		parsed, err := time.ParseDuration(*tickRate)
		if err != nil {
			slog.Error("failed to parse tick rate", "err", err)
			os.Exit(1)
		}
		interval = parsed
	}

	logger := slog.Default()

	simCfg := simulation.Config{NumTrucks: *trucks, UpdateInterval: interval}
	if *boundingBox != "" {
		bbox, err := parseBoundingBox(*boundingBox)
		if err != nil {
			logger.Error("failed to parse bounding box", "err", err)
			os.Exit(1)
		}
		simCfg.RouteBounds = []simulation.BoundingBox{bbox}
	}
	sim := simulation.NewManager(simCfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sim.Start(ctx); err != nil {
		logger.Error("failed to start simulation", "err", err)
		os.Exit(1)
	}

	srv := server.NewServer(sim).WithLogger(logger)
	if *enableAdmin {
		srv = srv.WithAdminEnabled()
	}

	httpServer := &http.Server{Addr: *addr, Handler: srv.Routes()}

	go func() {
		logger.Info("starting server", "addr", *addr, "admin_enabled", *enableAdmin)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "err", err)
			cancel()
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signals:
		logger.Info("shutting down server")
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = httpServer.Shutdown(shutdownCtx)
	sim.Stop()
}

func envString(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		parsed, err := time.ParseDuration(val)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func parseBoundingBox(value string) (simulation.BoundingBox, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return simulation.BoundingBox{}, fmt.Errorf("expected 4 comma-separated values, got %d", len(parts))
	}

	toFloat := func(v string) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	}

	minLat, err := toFloat(parts[0])
	if err != nil {
		return simulation.BoundingBox{}, errors.New("invalid min latitude")
	}
	minLon, err := toFloat(parts[1])
	if err != nil {
		return simulation.BoundingBox{}, errors.New("invalid min longitude")
	}
	maxLat, err := toFloat(parts[2])
	if err != nil {
		return simulation.BoundingBox{}, errors.New("invalid max latitude")
	}
	maxLon, err := toFloat(parts[3])
	if err != nil {
		return simulation.BoundingBox{}, errors.New("invalid max longitude")
	}

	return simulation.BoundingBox{MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon}, nil
}
