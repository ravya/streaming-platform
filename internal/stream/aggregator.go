// Package stream is a small, in-process model of the stateful stream job
// (Go Kafka consumer-group workers + Redis state in production): de-duplication, session state and last-touch
// click-to-basket attribution, fanning out to hot and cold sinks.
package stream

import (
	"sync"
	"time"

	"github.com/ravya/retail-insights-platform/internal/domain"
	"github.com/ravya/retail-insights-platform/internal/store"
)

type touch struct {
	campaignID string
	at         time.Time
}

type Aggregator struct {
	sinks             []store.Writer
	attributionWindow time.Duration
	dedupTTL          time.Duration

	mu      sync.Mutex
	lastClk map[string]touch     // tenant|customer -> last click (keyed session state)
	seen    map[string]time.Time // event id -> first seen (idempotency / at-least-once -> effectively-once)
}

func NewAggregator(attribution, dedupTTL time.Duration, sinks ...store.Writer) *Aggregator {
	return &Aggregator{
		sinks: sinks, attributionWindow: attribution, dedupTTL: dedupTTL,
		lastClk: map[string]touch{}, seen: map[string]time.Time{},
	}
}

// Process handles one event. Returns false if it was a duplicate or unattributable.
func (a *Aggregator) Process(e domain.Event) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, dup := a.seen[e.TenantID+"|"+e.ID]; dup {
		return false
	}
	a.seen[e.TenantID+"|"+e.ID] = e.Timestamp

	sessKey := e.TenantID + "|" + e.CustomerID
	switch e.Type {
	case domain.EventImpression:
		a.emit(domain.MetricImpressions, e.TenantID, e.CampaignID, e.CustomerID, e.Timestamp)
	case domain.EventClick:
		a.lastClk[sessKey] = touch{e.CampaignID, e.Timestamp}
		a.emit(domain.MetricClicks, e.TenantID, e.CampaignID, e.CustomerID, e.Timestamp)
	case domain.EventAddToCart:
		t, ok := a.lastClk[sessKey]
		if !ok || e.Timestamp.Sub(t.at) > a.attributionWindow || e.Timestamp.Before(t.at) {
			return false // no click inside the attribution window
		}
		a.emit(domain.MetricClickToBasket, e.TenantID, t.campaignID, e.CustomerID, e.Timestamp)
	}
	return true
}

func (a *Aggregator) emit(m domain.Metric, tenant, campaign, customer string, ts time.Time) {
	for _, s := range a.sinks {
		s.Record(m, tenant, campaign, customer, ts)
	}
}

// Sweep expires session/dedup state (state TTL keeps memory bounded).
func (a *Aggregator) Sweep(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, t := range a.lastClk {
		if now.Sub(t.at) > a.attributionWindow {
			delete(a.lastClk, k)
		}
	}
	for k, t := range a.seen {
		if now.Sub(t) > a.dedupTTL {
			delete(a.seen, k)
		}
	}
}
