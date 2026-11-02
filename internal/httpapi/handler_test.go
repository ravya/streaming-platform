package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ravya/retail-insights-platform/internal/auth"
	"github.com/ravya/retail-insights-platform/internal/domain"
	"github.com/ravya/retail-insights-platform/internal/metrics"
	"github.com/ravya/retail-insights-platform/internal/resilience"
	"github.com/ravya/retail-insights-platform/internal/service"
	"github.com/ravya/retail-insights-platform/internal/store"
	"github.com/ravya/retail-insights-platform/internal/tenant"
)

var secret = []byte("test-secret")

type env struct {
	srv       *httptest.Server
	hot, cold *store.Memory
	reg       *metrics.Registry
}

func setup(t *testing.T) env {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hot, cold := store.NewMemory("hot", time.Minute, store.Union), store.NewMemory("cold", 24*time.Hour, store.SumOfBuckets)
	for _, s := range []*store.Memory{hot, cold} {
		s.RegisterCampaign("acme", "c1")
		s.RegisterCampaign("globex", "g1")
	}
	reg := metrics.NewRegistry()
	svc := service.NewInsights(hot, cold, resilience.NewBreaker(2, time.Minute), service.DefaultOptions(), log, nil)
	h := NewRouter(Deps{Svc: svc, Tenants: tenant.NewRegistry(
		tenant.Config{ID: "acme", RPS: 1000, Burst: 1000}, tenant.Config{ID: "globex", RPS: 0.001, Burst: 2}),
		Metrics: reg, Log: log, JWTSecret: secret})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return env{srv, hot, cold, reg}
}

func token(tn string, scopes ...string) string {
	return auth.Sign(secret, auth.Claims{Tenant: tn, Scopes: scopes, Exp: time.Now().Add(time.Hour).Unix()})
}

func get(t *testing.T, e env, path, tok string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestHappyPathAndRouting(t *testing.T) {
	e := setup(t)
	now := time.Now()
	for _, c := range []string{"u1", "u2", "u1"} { // 2 unique customers
		e.hot.Record(domain.MetricClicks, "acme", "c1", c, now.Add(-time.Hour))
		e.cold.Record(domain.MetricClicks, "acme", "c1", c, now.Add(-time.Hour))
	}
	code, body := get(t, e, "/v1/ad/c1/clicks", token("acme", "insights:read"))
	if code != 200 || body["count"].(float64) != 2 || body["source"] != "realtime" {
		t.Fatalf("got %d %v", code, body)
	}
	old := now.Add(-10 * 24 * time.Hour)
	e.cold.Record(domain.MetricClicks, "acme", "c1", "u9", old)
	q := "?from=" + old.Add(-time.Hour).UTC().Format(time.RFC3339) + "&to=" + old.Add(time.Hour).UTC().Format(time.RFC3339)
	code, body = get(t, e, "/v1/ad/c1/clicks"+q, token("acme", "insights:read"))
	if code != 200 || body["count"].(float64) != 1 || body["source"] != "historical" {
		t.Fatalf("historical: got %d %v", code, body)
	}
}

func TestSecurityAndTenancy(t *testing.T) {
	e := setup(t)
	cases := []struct {
		name, path, tok string
		want            int
	}{
		{"no token", "/v1/ad/c1/clicks", "", 401},
		{"bad token", "/v1/ad/c1/clicks", "junk", 401},
		{"missing scope", "/v1/ad/c1/clicks", token("acme"), 403},
		{"unknown tenant", "/v1/ad/c1/clicks", token("evil", "insights:read"), 403},
		{"other tenant's campaign is 404", "/v1/ad/g1/clicks", token("acme", "insights:read"), 404},
		{"bad campaign id", "/v1/ad/c1;drop/clicks", token("acme", "insights:read"), 400},
		{"bad time", "/v1/ad/c1/clicks?from=yesterday", token("acme", "insights:read"), 400},
		{"range too large", "/v1/ad/c1/clicks?from=2000-01-01T00:00:00Z", token("acme", "insights:read"), 400},
	}
	for _, c := range cases {
		if code, _ := get(t, e, c.path, c.tok); code != c.want {
			t.Errorf("%s: got %d want %d", c.name, code, c.want)
		}
	}
}

func TestPerTenantRateLimit(t *testing.T) {
	e := setup(t)
	tok := token("globex", "insights:read") // burst=2
	var codes []int
	for i := 0; i < 4; i++ {
		c, _ := get(t, e, "/v1/ad/g1/clicks", tok)
		codes = append(codes, c)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[3] != 429 {
		t.Fatalf("unexpected codes %v", codes)
	}
	// acme is unaffected by globex's exhaustion (noisy-neighbour isolation)
	if c, _ := get(t, e, "/v1/ad/c1/clicks", token("acme", "insights:read")); c != 200 {
		t.Fatalf("acme got %d", c)
	}
}

func TestDegradedFallbackWhenHotStoreFails(t *testing.T) {
	e := setup(t)
	e.cold.Record(domain.MetricClicks, "acme", "c1", "u1", time.Now().Add(-30*time.Minute))
	e.hot.SetFailing(true)
	code, body := get(t, e, "/v1/ad/c1/clicks", token("acme", "insights:read"))
	if code != 200 || body["degraded"] != true || body["source"] != "historical" || body["count"].(float64) != 1 {
		t.Fatalf("expected degraded fallback, got %d %v", code, body)
	}
}

func TestMetricsAndHealth(t *testing.T) {
	e := setup(t)
	get(t, e, "/v1/ad/c1/clicks", token("acme", "insights:read"))
	for _, p := range []string{"/healthz", "/readyz"} {
		if c, _ := get(t, e, p, ""); c != 200 {
			t.Errorf("%s -> %d", p, c)
		}
	}
	resp, _ := http.Get(e.srv.URL + "/metrics")
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `http_requests_total{route="GET /v1/ad/{campaignID}/clicks",status="200"} 1`) {
		t.Fatalf("metrics missing RED counter:\n%s", b)
	}
}

func TestHistoricalBasisSingleDayVsMultiDay(t *testing.T) {
	e := setup(t)
	day := time.Now().UTC().Truncate(24 * time.Hour).Add(-10 * 24 * time.Hour)
	e.cold.Record(domain.MetricClicks, "acme", "c1", "u1", day.Add(2*time.Hour))
	e.cold.Record(domain.MetricClicks, "acme", "c1", "u1", day.Add(-22*time.Hour)) // same customer, previous day
	e.cold.Record(domain.MetricClicks, "acme", "c1", "u2", day.Add(5*time.Hour))
	f := func(from, to time.Time) string {
		return "?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	}
	tok := token("acme", "insights:read")
	_, one := get(t, e, "/v1/ad/c1/clicks"+f(day, day.Add(24*time.Hour)), tok)
	if one["count"].(float64) != 2 || one["basis"] != "unique_customers" {
		t.Fatalf("single day: %v", one)
	}
	_, two := get(t, e, "/v1/ad/c1/clicks"+f(day.Add(-24*time.Hour), day.Add(24*time.Hour)), tok)
	if two["count"].(float64) != 3 || two["basis"] != "sum_of_daily_unique" { // u1 counted on both days
		t.Fatalf("two days: %v", two)
	}
}
