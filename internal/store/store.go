// Package store defines the read-side storage port and an in-memory adapter.
//
// In production the "hot" adapter is Redis Sets (SADD / SUNION / SCARD over
// minute and hour buckets, 48 h TTL) and the "cold" adapter is DynamoDB items
// holding the exact per-day unique count precomputed by the nightly batch job.
// The in-memory adapter keeps the same contract so the service layer is
// identical in both worlds.
package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ravya/retail-insights-platform/internal/domain"
)

// Reader is the port used by the service layer.
type Reader interface {
	Name() string
	UniqueCustomers(ctx context.Context, q domain.Query) (int64, error)
	CampaignExists(ctx context.Context, tenantID, campaignID string) (bool, error)
}

// Writer is the port used by the stream processor.
type Writer interface {
	Record(m domain.Metric, tenantID, campaignID, customerID string, ts time.Time)
}

// Mode says how bucket results are combined for a range query.
type Mode int

const (
	// Union counts distinct customers across ALL buckets in the range (exact).
	// Needs the member sets, so it is used for the short hot window (Redis Sets).
	Union Mode = iota
	// SumOfBuckets adds each bucket's own distinct count. Exact within a bucket
	// (a day) but a customer active on two days counts twice ("customer-days").
	// This is what a precomputed per-day number in DynamoDB can offer.
	SumOfBuckets
)

type series struct {
	tenant, campaign string
	metric           domain.Metric
}

// Memory is a bucketed unique-customer store with exact sets per bucket.
type Memory struct {
	name string
	gran time.Duration
	mode Mode

	mu        sync.RWMutex
	data      map[series]map[int64]map[string]struct{} // series -> bucketStart(unix) -> customers
	campaigns map[string]struct{}                      // tenant|campaign registry

	fail atomic.Bool // chaos switch for resilience tests
}

func NewMemory(name string, gran time.Duration, mode Mode) *Memory {
	return &Memory{
		name:      name,
		gran:      gran,
		mode:      mode,
		data:      map[series]map[int64]map[string]struct{}{},
		campaigns: map[string]struct{}{},
	}
}

func (m *Memory) Name() string             { return m.name }
func (m *Memory) SetFailing(f bool)        { m.fail.Store(f) }
func (m *Memory) bucket(t time.Time) int64 { return t.Truncate(m.gran).Unix() }

// RegisterCampaign records that a campaign belongs to a tenant (campaign metadata).
func (m *Memory) RegisterCampaign(tenantID, campaignID string) {
	m.mu.Lock()
	m.campaigns[tenantID+"|"+campaignID] = struct{}{}
	m.mu.Unlock()
}

// Record is idempotent: adding the same customer to a bucket twice changes nothing
// (like Redis SADD), so replays after a crash cannot double count.
func (m *Memory) Record(metric domain.Metric, tenantID, campaignID, customerID string, ts time.Time) {
	k := series{tenantID, campaignID, metric}
	b := m.bucket(ts)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[k] == nil {
		m.data[k] = map[int64]map[string]struct{}{}
	}
	if m.data[k][b] == nil {
		m.data[k][b] = map[string]struct{}{}
	}
	m.data[k][b][customerID] = struct{}{}
}

var errInjected = errors.New("injected store failure")

// UniqueCustomers counts customers over the buckets covering [From, To).
// Tenant is part of the series key, so a query can never read another tenant's data.
func (m *Memory) UniqueCustomers(ctx context.Context, q domain.Query) (int64, error) {
	if m.fail.Load() {
		return 0, errInjected
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	buckets := m.data[series{q.TenantID, q.CampaignID, q.Metric}]
	union := map[string]struct{}{}
	var sum int64
	for start := q.From.Truncate(m.gran); start.Before(q.To); start = start.Add(m.gran) {
		set := buckets[start.Unix()]
		if m.mode == SumOfBuckets {
			sum += int64(len(set))
			continue
		}
		for c := range set {
			union[c] = struct{}{}
		}
	}
	if m.mode == SumOfBuckets {
		return sum, nil
	}
	return int64(len(union)), nil
}

func (m *Memory) CampaignExists(_ context.Context, tenantID, campaignID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.campaigns[tenantID+"|"+campaignID]
	return ok, nil
}

// Evict drops buckets older than retention (hot store TTL / data-lifecycle policy).
func (m *Memory) Evict(now time.Time, retention time.Duration) {
	cutoff := now.Add(-retention).Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, buckets := range m.data {
		for b := range buckets {
			if b < cutoff {
				delete(buckets, b)
			}
		}
	}
}
