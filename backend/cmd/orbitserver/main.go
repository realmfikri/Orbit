package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orbit/backend/server"
	"orbit/backend/simulation"
)

func main() {
	var (
		addr           = flag.String("addr", ":8080", "HTTP listen address")
		enableAdmin    = flag.Bool("enable-admin", false, "enable admin endpoints like pprof")
		trucks         = flag.Int("trucks", 2000, "number of trucks to simulate")
		updateInterval = flag.Duration("update-interval", time.Second, "simulation update interval")
	)
	flag.Parse()

	logger := slog.Default()

	simCfg := simulation.Config{NumTrucks: *trucks, UpdateInterval: *updateInterval}
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
