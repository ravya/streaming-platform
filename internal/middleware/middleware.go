// Package middleware holds the cross-cutting HTTP concerns: request IDs,
// structured logging, metrics, panic recovery, authN/Z, per-tenant rate
// limiting, timeouts and security headers.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ravya/retail-insights-platform/internal/auth"
	"github.com/ravya/retail-insights-platform/internal/metrics"
	"github.com/ravya/retail-insights-platform/internal/tenant"
)

type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first listed is the outermost.
func Chain(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

type ctxKey int

const (
	reqIDKey ctxKey = iota
	claimsKey
)

func RequestIDFrom(ctx context.Context) string { s, _ := ctx.Value(reqIDKey).(string); return s }
func ClaimsFrom(ctx context.Context) (auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(auth.Claims)
	return c, ok
}

// WriteError emits a uniform JSON error envelope.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg, "requestId": RequestIDFrom(r.Context())}})
}

// RequestID propagates X-Request-ID, or the trace-id of a W3C traceparent header,
// so one ID ties logs, metrics exemplars and traces together.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if tp := strings.Split(r.Header.Get("traceparent"), "-"); id == "" && len(tp) == 4 {
			id = tp[1]
		}
		if id == "" || len(id) > 64 {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), reqIDKey, id)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

// Observe records access logs + RED metrics (Rate, Errors, Duration). Place OUTSIDE the mux
// so the matched route pattern (low-cardinality label) is available afterwards.
func Observe(log *slog.Logger, reg *metrics.Registry) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			d := time.Since(start)
			reg.Inc("http_requests_total", "route", route, "status", strconv.Itoa(sw.status))
			reg.Observe("http_request_duration_seconds", d.Seconds(), "route", route)
			lvl := slog.LevelInfo
			if sw.status >= 500 {
				lvl = slog.LevelError
			}
			log.Log(r.Context(), lvl, "request", "method", r.Method, "route", route, "status", sw.status,
				"duration_ms", d.Milliseconds(), "request_id", RequestIDFrom(r.Context()))
		})
	}
}

func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					log.Error("panic", "panic", p, "request_id", RequestIDFrom(r.Context()))
					WriteError(w, r, 500, "internal", "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authenticate verifies the bearer JWT, checks scope and that the tenant is known.
// The tenant comes ONLY from the verified token, never from the URL or a header.
func Authenticate(secret []byte, scope string, tenants *tenant.Registry) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				WriteError(w, r, 401, "unauthenticated", "missing bearer token")
				return
			}
			c, err := auth.Verify(secret, tok, time.Now())
			if err != nil {
				WriteError(w, r, 401, "unauthenticated", "invalid or expired token")
				return
			}
			if _, known := tenants.Get(c.Tenant); !known || !c.HasScope(scope) {
				WriteError(w, r, 403, "forbidden", "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, c)))
		})
	}
}

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimit is a per-tenant token bucket using tenant-specific quotas
// ("noisy neighbour" protection). In a multi-pod deployment, enforce at the
// gateway or back it with Redis for a global limit.
func RateLimit(tenants *tenant.Registry, reg *metrics.Registry) Middleware {
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, _ := ClaimsFrom(r.Context())
			cfg, _ := tenants.Get(c.Tenant)
			now := time.Now()
			mu.Lock()
			b := buckets[c.Tenant]
			if b == nil {
				b = &bucket{tokens: float64(cfg.Burst), last: now}
				buckets[c.Tenant] = b
			}
			b.tokens = min(float64(cfg.Burst), b.tokens+now.Sub(b.last).Seconds()*cfg.RPS)
			b.last = now
			allowed := b.tokens >= 1
			if allowed {
				b.tokens--
			}
			mu.Unlock()
			if !allowed {
				reg.Inc("rate_limited_total", "tenant", c.Tenant)
				w.Header().Set("Retry-After", "1")
				WriteError(w, r, 429, "rate_limited", "tenant quota exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
