package stream

import (
	"context"
	"testing"
	"time"

	"github.com/ravya/retail-insights-platform/internal/domain"
	"github.com/ravya/retail-insights-platform/internal/store"
)

func TestAttributionDedupAndWindow(t *testing.T) {
	st := store.NewMemory("t", time.Minute, store.Union)
	a := NewAggregator(30*time.Minute, time.Hour, st)
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ev := func(id string, typ domain.EventType, camp, cust string, at time.Time) domain.Event {
		return domain.Event{ID: id, TenantID: "a", CampaignID: camp, CustomerID: cust, Type: typ, Timestamp: at}
	}
	a.Process(ev("1", domain.EventClick, "c1", "u1", t0))
	if !a.Process(ev("2", domain.EventAddToCart, "", "u1", t0.Add(5*time.Minute))) {
		t.Fatal("atc within window should attribute")
	}
	if a.Process(ev("2", domain.EventAddToCart, "", "u1", t0.Add(5*time.Minute))) {
		t.Fatal("duplicate event id must be dropped")
	}
	if a.Process(ev("3", domain.EventAddToCart, "", "u1", t0.Add(45*time.Minute))) {
		t.Fatal("atc outside window must not attribute")
	}
	if a.Process(ev("4", domain.EventAddToCart, "", "u2", t0)) {
		t.Fatal("atc without prior click must not attribute")
	}
	n, _ := st.UniqueCustomers(context.Background(), domain.Query{TenantID: "a", CampaignID: "c1", Metric: domain.MetricClickToBasket, From: t0, To: t0.Add(time.Hour)})
	if n != 1 {
		t.Fatalf("clickToBasket=%d want 1", n)
	}
}
