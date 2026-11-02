// Package httpapi is the transport layer: routing, parameter parsing and
// mapping domain errors to HTTP status codes. No business logic lives here.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/ravya/retail-insights-platform/internal/domain"
	"github.com/ravya/retail-insights-platform/internal/metrics"
	"github.com/ravya/retail-insights-platform/internal/middleware"
	"github.com/ravya/retail-insights-platform/internal/service"
	"github.com/ravya/retail-insights-platform/internal/tenant"
)

var campaignRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`) // input validation / injection guard

type Deps struct {
	Svc       *service.Insights
	Tenants   *tenant.Registry
	Metrics   *metrics.Registry
	Log       *slog.Logger
	JWTSecret []byte
	Ready     func() bool
}

const scope = "insights:read"

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	secured := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h,
			middleware.Authenticate(d.JWTSecret, scope, d.Tenants),
			middleware.RateLimit(d.Tenants, d.Metrics),
			middleware.Timeout(2*time.Second),
		)
	}
	// Metric names follow the assignment: clicks | impressions | clickToBasket.
	mux.Handle("GET /v1/ad/{campaignID}/clicks", secured(d.metricHandler(domain.MetricClicks)))
	mux.Handle("GET /v1/ad/{campaignID}/impressions", secured(d.metricHandler(domain.MetricImpressions)))
	mux.Handle("GET /v1/ad/{campaignID}/clickToBasket", secured(d.metricHandler(domain.MetricClickToBasket)))

	// Operational endpoints (not tenant-facing; restrict via NetworkPolicy / separate port in prod).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if d.Ready != nil && !d.Ready() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ready"))
	})
	mux.Handle("GET /metrics", d.Metrics.Handler())

	return middleware.Chain(mux,
		middleware.RequestID,
		middleware.Observe(d.Log, d.Metrics),
		middleware.Recover(d.Log),
		middleware.SecurityHeaders,
	)
}

func (d Deps) metricHandler(m domain.Metric) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, _ := middleware.ClaimsFrom(r.Context())
		id := r.PathValue("campaignID")
		if !campaignRe.MatchString(id) {
			middleware.WriteError(w, r, 400, "invalid_request", "invalid campaignID")
			return
		}
		now := time.Now().UTC().Truncate(time.Minute) // minute-aligned => cacheable keys
		from, to := now.Add(-24*time.Hour), now.Add(time.Minute)
		var err error
		if v := r.URL.Query().Get("from"); v != "" {
			if from, err = time.Parse(time.RFC3339, v); err != nil {
				middleware.WriteError(w, r, 400, "invalid_request", "from must be RFC3339")
				return
			}
		}
		if v := r.URL.Query().Get("to"); v != "" {
			if to, err = time.Parse(time.RFC3339, v); err != nil {
				middleware.WriteError(w, r, 400, "invalid_request", "to must be RFC3339")
				return
			}
		}
		res, err := d.Svc.Get(r.Context(), domain.Query{TenantID: claims.Tenant, CampaignID: id, Metric: m, From: from.UTC(), To: to.UTC()})
		if err != nil {
			d.writeErr(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}
}

func (d Deps) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		middleware.WriteError(w, r, 404, "not_found", "campaign not found")
	case errors.Is(err, domain.ErrInvalid):
		middleware.WriteError(w, r, 400, "invalid_request", err.Error())
	default: // never leak internals to callers; details are in logs
		d.Log.Error("request failed", "err", err, "request_id", middleware.RequestIDFrom(r.Context()))
		middleware.WriteError(w, r, 503, "unavailable", "temporarily unavailable, retry later")
	}
}
