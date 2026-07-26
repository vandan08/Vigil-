// Command vigil runs the Vigil incident response server.
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

	"github.com/vandan08/vigil/internal/alert"
	"github.com/vandan08/vigil/internal/api"
	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/ingest"
	"github.com/vandan08/vigil/internal/metrics"
	"github.com/vandan08/vigil/internal/notify"
	"github.com/vandan08/vigil/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store := incident.NewMemoryStore()

	// Self-metrics (ADR-005): scraped at GET /metrics by the compose stack's
	// Prometheus. Gauges owned elsewhere are polled at scrape time.
	m := metrics.New()
	m.OpenIncidents = func() int {
		n := 0
		for _, inc := range store.List() {
			if inc.State != incident.StateResolved {
				n++
			}
		}
		return n
	}

	// Every lifecycle event reaches the SSE bus feeding dashboard clients.
	// Slack is opt-in: set VIGIL_SLACK_WEBHOOK_URL to an incoming-webhook
	// URL. Delivery is async and lossy under pressure on both paths.
	bus := notify.NewBus()
	emit := bus.Publish
	if url := os.Getenv("VIGIL_SLACK_WEBHOOK_URL"); url != "" {
		dispatcher := notify.NewDispatcher(log, notify.NewSlackWebhook(url), 64)
		defer dispatcher.Close() // runs after Shutdown: drain, then exit
		m.DroppedNotifications = dispatcher.Dropped
		emit = func(ev notify.Event) {
			bus.Publish(ev)
			dispatcher.Enqueue(ev)
		}
		log.Info("slack notifications enabled")
	}

	handler := ingest.NewHandler(log, alert.NewDeduper(5*time.Minute), store)
	handler.Notify = emit
	handler.Metrics = m
	apiHandler := api.NewHandler(log, store, bus)
	apiHandler.Notify = emit

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.NewMux(handler, apiHandler, m),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("vigil listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Info("shutting down")
	_ = srv.Shutdown(shutdownCtx)
}
