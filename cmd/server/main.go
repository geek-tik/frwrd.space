package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/forward/forward/internal/config"
	"github.com/forward/forward/internal/inspector"
	"github.com/forward/forward/internal/registry"
	"github.com/forward/forward/internal/server"
	"github.com/forward/forward/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.LoadServer()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	reg := registry.New()
	inspectorHub := inspector.NewHub()
	inspectorSvc := inspector.NewService(store.NewHTTPRequestStore(pool), inspectorHub)

	httpSrv := server.NewHTTPServer(cfg, pool, reg, inspectorSvc)
	edgeSrv := server.NewEdgeServer(cfg, reg, inspectorSvc)
	agentSrv := server.NewAgentServer(cfg, pool, reg)

	servers := []*http.Server{
		{Addr: cfg.HTTPAddr, Handler: httpSrv.Router()},
		{Addr: cfg.EdgeAddr, Handler: edgeSrv.Handler()},
		{Addr: cfg.AgentAddr, Handler: agentSrv.Handler()},
	}

	errCh := make(chan error, len(servers))
	for _, srv := range servers {
		srv := srv
		go func() {
			slog.Info("listening", "addr", srv.Addr)
			errCh <- srv.ListenAndServe()
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case sig := <-stop:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "addr", srv.Addr, "error", err)
		}
	}
}
