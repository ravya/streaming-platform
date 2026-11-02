# Retail Ad Insights Platform: case study

Real-time streaming platform for an online shopping network: ingest shopper/ad events, process them with stateful stream jobs, and serve campaign insights through APIs.

* **Design doc:** [`docs/DESIGN.md`](docs/DESIGN.md) (Word: `docs/Design_Proposal.docx`)
* **Assumptions and sizing:** [`docs/ASSUMPTIONS.md`](docs/ASSUMPTIONS.md) (Word: `docs/Assumptions_and_Sizing.docx`)
* **Diagrams:** [high-level design](docs/diagrams/hld.png) · [data flow](docs/diagrams/dataflow.png) · [deployment](docs/diagrams/deployment.png)
* **Code (this repo):** the **Insights API** service, plus a small stream aggregator that models the Go stream-worker logic.

## Run it

```bash
make test                         # vet + race detector + coverage
make run                          # http://localhost:8080 (demo data seeded in memory)
TOKEN=$(JWT_SECRET=dev-secret go run ./cmd/tokengen flipkart)
curl -H "Authorization: Bearer $TOKEN" localhost:8080/v1/ad/c-1001/clicks
curl -H "Authorization: Bearer $TOKEN" "localhost:8080/v1/ad/c-1001/clickToBasket?from=$(date -u -d '3 days ago' +%Y-%m-%dT00:00:00Z)&to=$(date -u -d '2 days ago' +%Y-%m-%dT00:00:00Z)"
curl localhost:8080/metrics
```
Demo tenants (`configs/tenants.json`): `flipkart` (campaigns `c-1001`, `c-1002`), `walmart` (`c-2001`). Asking for another tenant's campaign returns `404`.

## Code layout

```
cmd/insights-api      wiring, config, graceful shutdown
cmd/tokengen          mint dev JWTs
internal/domain       core types + errors (no deps)
internal/service      use case: validate, hot/cold routing, cache, breaker, fallback
internal/store        read/write ports + in-memory adapter (prod: Redis Sets / DynamoDB daily counts)
internal/stream       dedup, session state, click→cart attribution (prod: Go Kafka workers + Redis state)
internal/httpapi      routes, parsing, error mapping
internal/middleware   request-id, logging, RED metrics, recover, authN/Z, rate limit, timeout, headers
internal/{auth,cache,metrics,resilience,tenant}
deploy/k8s            Deployment, HPA, PDB, NetworkPolicy
```

## Cross-cutting concerns implemented (basic, working)

| Concern | Implementation |
|---|---|
| Security | JWT (pinned alg, exp, scope), tenant from token only, input validation, security headers, server timeouts, distroless non-root image |
| Multi-tenancy | tenant in store/cache keys, per-tenant quotas, cross-tenant access → 404 |
| Observability | structured JSON logs with request_id, Prometheus `/metrics` (RED, breaker, cache/fallback counters), `/healthz`, `/readyz`, traceparent propagation |
| Resilience | circuit breaker, timeouts, graceful degradation to cold store, graceful shutdown, request coalescing |
| Scaling | stateless service, HPA/PDB/topology spread manifests, cache |

## Production substitutions (same interfaces)

Redis Set and DynamoDB adapters for `store.Reader`; JWKS/RS256 verification; Redis-backed global rate limit; `prometheus/client_golang` and OpenTelemetry SDK; Kafka consumer-group workers backed by Redis in place of `internal/stream`.
