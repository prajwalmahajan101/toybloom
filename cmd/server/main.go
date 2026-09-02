package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prajwalmahajan101/toybloom/internal/api"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/core/logger"
	"github.com/prajwalmahajan101/toybloom/internal/service"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
	"github.com/valkey-io/valkey-go"
)

func main() {
	cfg := config.Load()
	lg := logger.New(cfg.LogLevel)
	slog.SetDefault(lg) // FromContext fallback + any pre-request panic log

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.ValkeyAddr},
	})
	if err != nil {
		lg.Error("valkey client init failed", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	vs := store.NewValkeyStore(client)
	svc := service.New(vs)
	h := api.NewHandler(svc)
	hh := api.NewHealthHandler(vs)
	r := api.NewRouter(h, hh, cfg, lg)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	// Trap SIGINT/SIGTERM; ctx is cancelled on the first signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run the server in the background so main can wait on the signal.
	go func() {
		lg.Info("listening", "port", cfg.Port, "valkey", cfg.ValkeyAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			lg.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Block until a shutdown signal arrives.
	<-ctx.Done()
	stop() // restore default signal handling so a second signal force-kills
	lg.Info("shutting down, draining in-flight requests")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	lg.Info("server stopped cleanly")
}
