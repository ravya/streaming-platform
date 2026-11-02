package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ravya/retail-insights-platform/internal/httpapi"
	"github.com/ravya/retail-insights-platform/internal/metrics"
	"github.com/ravya/retail-insights-platform/internal/resilience"
	"github.com/ravya/retail-insights-platform/internal/service"
	"github.com/ravya/retail-insights-platform/internal/store"
	"github.com/ravya/retail-insights-platform/internal/stream"
	"github.com/ravya/retail-insights-platform/internal/tenant"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Error("JWT_SECRET is required") // fail fast: never run with a default secret
		os.Exit(1)
	}
	tenants, err := tenant.Load(env("TENANTS_FILE", "configs/tenants.json"))
	if err != nil {
		log.Error("load tenants", "err", err)
		os.Exit(1)
	}

	reg := metrics.NewRegistry()
	opt := service.DefaultOptions()
	hot := store.NewMemory("hot", 5*time.Minute, store.Union)
	cold := store.NewMemory("cold", 24*time.Hour, store.SumOfBuckets)
	breaker := resilience.NewBreaker(5, 10*time.Second)
	reg.Gauge("circuit_breaker_open", func() float64 {
		if breaker.State() == resilience.Open {
			return 1
		}
		return 0
	})
	svc := service.NewInsights(hot, cold, breaker, opt, log, func(ev string) { reg.Inc("insights_events_total", "event", ev) })

	// Demo data: stands in for the Kafka -> Go workers pipeline.
	if env("SEED", "true") == "true" {
		campaigns := map[string][]string{"flipkart": {"c-1001", "c-1002"}, "walmart": {"c-2001"}}
		for t, cs := range campaigns {
			for _, c := range cs {
				hot.RegisterCampaign(t, c)
				cold.RegisterCampaign(t, c)
			}
		}
		agg := stream.NewAggregator(30*time.Minute, time.Hour, hot, cold)
		n := stream.Seed(agg, campaigns, 72*time.Hour, time.Now())
		hot.Evict(time.Now(), opt.HotRetention)
		log.Info("seeded demo events", "events", n)
	}

	var ready atomic.Bool
	ready.Store(true)
	srv := &http.Server{
		Addr: env("ADDR", ":8080"),
		Handler: httpapi.NewRouter(httpapi.Deps{
			Svc: svc, Tenants: tenants, Metrics: reg, Log: log, JWTSecret: []byte(secret), Ready: ready.Load,
		}),
		ReadHeaderTimeout: 3 * time.Second, // slowloris protection
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 14,
	}

	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown: flip readiness so the LB drains us, then finish in-flight requests.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ready.Store(false)
	log.Info("shutting down")
	time.Sleep(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
