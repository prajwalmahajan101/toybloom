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

	"github.com/prajwalmahajan101/toybloom/internal/api"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/core/logger"
	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"github.com/prajwalmahajan101/toybloom/internal/service"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
	"github.com/valkey-io/valkey-go"
)

// version labels telemetry via the OTel resource (service.version). Overridable
// at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg := config.Load()

	// Base logger for anything before the OTel SDK is up (client init, obs setup
	// failure). Replaced below by the otelslog-backed logger once obs is ready.
	slog.SetDefault(logger.New(cfg.LogLevel))

	// Bring up observability first so every downstream span/metric/log is
	// captured. ctx is the process lifetime; obs uses it for exporter dial-up.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	providers, err := obs.Setup(ctx, obs.Config{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: version,
		Exporter:       cfg.ObsExporter,
	})
	if err != nil {
		slog.Error("observability setup failed", "err", err)
		os.Exit(1)
	}
	lg := providers.Log
	slog.SetDefault(lg) // FromContext fallback + any pre-request panic log

	inst, err := obs.NewInstruments()
	if err != nil {
		lg.Error("observability instruments failed", "err", err)
		os.Exit(1)
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.ValkeyAddr},
	})
	if err != nil {
		lg.Error("valkey client init failed", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	vs := store.NewValkeyStore(client)
	svc := service.New(vs, inst, cfg.ObsMaxFilterGauges)

	// Register the async fill_ratio/estimated_fpp gauges. The callback reads the
	// live-filter view through svc each collection; harmless in no-op exporter
	// modes (nothing is registered).
	if err := inst.ObserveFilters(svc.FilterSamples); err != nil {
		lg.Error("observability filter gauges failed", "err", err)
		os.Exit(1)
	}

	h := api.NewHandler(svc)
	hh := api.NewHealthHandler(vs)
	r := api.NewRouter(h, hh, cfg, lg, inst)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	// Run the server in the background so main can wait on the signal (ctx is
	// cancelled on the first SIGINT/SIGTERM, set up above before obs).
	go func() {
		lg.Info("listening", "port", cfg.Port, "valkey", cfg.ValkeyAddr, "exporter", cfg.ObsExporter)
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

	// Order matters: drain the HTTP server first so in-flight requests finish
	// and their spans/metrics are recorded, THEN flush the telemetry exporters.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	if err := providers.Shutdown(flushCtx); err != nil {
		lg.Error("telemetry flush failed", "err", err)
	}
	lg.Info("server stopped cleanly")
}
