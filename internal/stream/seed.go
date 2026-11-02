package stream

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ravya/retail-insights-platform/internal/domain"
)

// Seed generates demo traffic for the given tenant->campaigns map over the last `span`.
func Seed(a *Aggregator, campaigns map[string][]string, span time.Duration, now time.Time) int {
	r := rand.New(rand.NewSource(42))
	n := 0
	for tenant, cs := range campaigns {
		for _, c := range cs {
			for i := 0; i < 4000; i++ {
				cust := fmt.Sprintf("cust-%d", r.Intn(1500))
				ts := now.Add(-time.Duration(r.Int63n(int64(span))))
				a.Process(domain.Event{ID: fmt.Sprintf("%s-%s-imp-%d", tenant, c, i), TenantID: tenant, CampaignID: c, CustomerID: cust, Type: domain.EventImpression, Timestamp: ts})
				n++
				if r.Float64() < 0.12 { // ~12% CTR
					cts := ts.Add(time.Duration(r.Intn(60)) * time.Second)
					a.Process(domain.Event{ID: fmt.Sprintf("%s-%s-clk-%d", tenant, c, i), TenantID: tenant, CampaignID: c, CustomerID: cust, Type: domain.EventClick, Timestamp: cts})
					n++
					if r.Float64() < 0.3 {
						a.Process(domain.Event{ID: fmt.Sprintf("%s-%s-atc-%d", tenant, c, i), TenantID: tenant, CustomerID: cust, Type: domain.EventAddToCart, Timestamp: cts.Add(time.Duration(r.Intn(600)) * time.Second)})
						n++
					}
				}
			}
		}
	}
	return n
}
