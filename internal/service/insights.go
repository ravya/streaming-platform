// Package service holds the use-case logic: validation, hot/cold routing,
// caching, circuit-breaking and graceful degradation.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ravya/retail-insights-platform/internal/cache"
	"github.com/ravya/retail-insights-platform/internal/domain"
	"github.com/ravya/retail-insights-platform/internal/resilience"
	"github.com/ravya/retail-insights-platform/internal/store"
)

type Options struct {
	HotRetention  time.Duration // data newer than this is served by the hot store
	MaxRange      time.Duration // guardrail against expensive scans
	RealtimeTTL   time.Duration
	HistoricalTTL time.Duration
	StoreTimeout  time.Duration
}

func DefaultOptions() Options {
	return Options{48 * time.Hour, 90 * 24 * time.Hour, 2 * time.Second, time.Minute, 500 * time.Millisecond}
}

type Insights struct {
	hot, cold store.Reader
	breaker   *resilience.Breaker
	cache     *cache.TTL[domain.Result]
	opt       Options
	now       func() time.Time
	log       *slog.Logger
	onEvent   func(name string) // metrics hook: cache_hit, fallback, ...
}

func NewInsights(hot, cold store.Reader, b *resilience.Breaker, opt Options, log *slog.Logger, onEvent func(string)) *Insights {
	if onEvent == nil {
		onEvent = func(string) {}
	}
	return &Insights{hot: hot, cold: cold, breaker: b, cache: cache.New[domain.Result](10_000), opt: opt, now: time.Now, log: log, onEvent: onEvent}
}

func (s *Insights) Get(ctx context.Context, q domain.Query) (domain.Result, error) {
	if err := s.validate(q); err != nil {
		return domain.Result{}, err
	}
	// Campaign metadata is the tenant-ownership check: another tenant's campaign is a 404, not a 403,
	// so campaign IDs cannot be enumerated across tenants.
	ok, err := s.cold.CampaignExists(ctx, q.TenantID, q.CampaignID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}
	if !ok {
		return domain.Result{}, domain.ErrNotFound
	}

	realtime := !q.From.Before(s.now().Add(-s.opt.HotRetention))
	ttl := s.opt.HistoricalTTL
	if realtime {
		ttl = s.opt.RealtimeTTL
	}
	// Tenant is part of the cache key: no cross-tenant cache leakage.
	key := fmt.Sprintf("%s|%s|%s|%d|%d", q.TenantID, q.CampaignID, q.Metric, q.From.Unix(), q.To.Unix())
	res, hit, err := s.cache.Do(key, ttl, func() (domain.Result, error) { return s.query(ctx, q, realtime) })
	if hit {
		s.onEvent("cache_hit")
	}
	return res, err
}

func (s *Insights) query(ctx context.Context, q domain.Query, realtime bool) (domain.Result, error) {
	res := domain.Result{CampaignID: q.CampaignID, Metric: q.Metric, From: q.From, To: q.To, AsOf: s.now().UTC()}
	if realtime {
		var n int64
		err := s.breaker.Do(func() error {
			cctx, cancel := context.WithTimeout(ctx, s.opt.StoreTimeout)
			defer cancel()
			var e error
			n, e = s.hot.UniqueCustomers(cctx, q)
			return e
		})
		if err == nil {
			res.Count, res.Source, res.Basis = n, "realtime", "unique_customers"
			return res, nil
		}
		// Graceful degradation: fall back to (slightly stale) rollups rather than fail.
		s.onEvent("fallback")
		s.log.Warn("hot store failed, falling back to cold", "err", err, "breaker", s.breaker.State().String())
		res.Degraded = true
	}
	cctx, cancel := context.WithTimeout(ctx, s.opt.StoreTimeout*4)
	defer cancel()
	n, err := s.cold.UniqueCustomers(cctx, q)
	if err != nil {
		return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}
	res.Count, res.Source = n, "historical"
	// History is stored as one exact unique count per day. A single-day range is exact;
	// a multi-day range is the SUM of daily uniques, so a customer active on 2 days counts twice.
	res.Basis = "sum_of_daily_unique"
	if !q.From.Truncate(24 * time.Hour).Add(24 * time.Hour).Before(q.To) {
		res.Basis = "unique_customers"
	}
	return res, nil
}

func (s *Insights) validate(q domain.Query) error {
	switch {
	case q.TenantID == "":
		return fmt.Errorf("%w: missing tenant", domain.ErrInvalid)
	case !q.From.Before(q.To):
		return fmt.Errorf("%w: from must be before to", domain.ErrInvalid)
	case q.To.Sub(q.From) > s.opt.MaxRange:
		return fmt.Errorf("%w: range exceeds %s", domain.ErrInvalid, s.opt.MaxRange)
	case q.To.After(s.now().Add(time.Minute)):
		return fmt.Errorf("%w: to is in the future", domain.ErrInvalid)
	}
	return nil
}

// IsBreakerOpen helps callers/tests distinguish degraded mode.
func IsBreakerOpen(err error) bool { return errors.Is(err, resilience.ErrOpen) }
