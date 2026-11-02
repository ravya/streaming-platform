from svgkit import SVG
W, H = 1560, 640
g = SVG(W, H, "End-to-End Data Flow")
cw, gap, x0 = 188, 28, 24

def row(y, label, steps, kind_map, tag):
    g.lane(14, y, W - 28, 150, label)
    for i, (t, l, k) in enumerate(steps):
        x = x0 + i * (cw + gap)
        g.box(x, y + 40, cw, 84, t, l, k, 12)
        g.badge(x + 14, y + 38, tag(i), "#0F172A")
        if i < len(steps) - 1:
            g.arrow([(x + cw, y + 82), (x + cw + gap, y + 82)])

row(52, "Event path: click to queryable (under 10 s)", [
    ("Shopper clicks ad", ["SDK batches events"], "src"),
    ("Edge", ["WAF, TLS, API key"], "edge"),
    ("Collector", ["validate, hash PII, 202"], "ingest"),
    ("Kafka", ["key = tenant:customer"], "kafka"),
    ("Worker A", ["drop duplicates"], "stream"),
    ("Worker B", ["match cart to last click"], "stream"),
    ("Worker C", ["add customer to Redis set"], "store"),
], None, lambda i: i + 1)

row(216, "Query path (under 200 ms)", [
    ("Client call", ["Bearer JWT, from/to"], "user"),
    ("Gateway", ["token, tenant, rate limit"], "edge"),
    ("Insights API", ["validate, own campaign"], "serve"),
    ("Cache", ["2 s, shared requests"], "serve"),
    ("Router", ["48 h Redis, else DynamoDB"], "serve"),
    ("Response", ["count, basis, source"], "ingest"),
], None, lambda i: "abcdef"[i])

row(380, "Nightly path: exact history", [
    ("S3 raw events", ["yesterday's files"], "store"),
    ("Nightly Go job", ["dedup, late events, bots"], "batch"),
    ("Exact count", ["per campaign, per day"], "batch"),
    ("DynamoDB", ["one row per day"], "store"),
    ("Drift check", ["alert if off by > 1%"], "batch"),
], None, lambda i: "n" + str(i + 1))

g.text(24, 570, "Kafka also writes every event to S3 (step 4), which the nightly job reads. Failure handling is in the failure scenarios section.", 12.5, "#334155")
g.save("../dataflow.svg")
