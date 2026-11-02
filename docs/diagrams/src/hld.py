from svgkit import SVG
W, H = 1560, 640
g = SVG(W, H, "High-Level Design")
cw, gap, x0 = 210, 36, 24
X = [x0 + i * (cw + gap) for i in range(6)]
R = X[5] + cw  # right edge of last column

g.lane(14, 52, W - 28, 150, "1 · Events come in")
Y1 = 88
g.box(X[0], Y1, cw, 84, "Retailer apps", ["web, mobile, ad servers"], "src", 12.5)
g.box(X[1], Y1, cw, 84, "Edge", ["WAF, TLS, API key"], "edge", 12.5)
g.box(X[2], Y1, cw, 84, "Collector (Go)", ["validate, return 202"], "ingest", 12.5)
g.box(X[3], Y1, cw, 84, "Kafka", ["buffer, replay, ordering"], "kafka", 12.5)
g.box(X[4], Y1, cw, 84, "Stream workers (Go)", ["dedup, attribute, count"], "stream", 12.5)
g.box(X[5], Y1, cw, 84, "Redis", ["last 48 h, exact sets"], "store", 12.5)
for i in range(5):
    g.arrow([(X[i] + cw, Y1 + 42), (X[i + 1], Y1 + 42)])

g.lane(14, 216, W - 28, 150, "2 · Nightly job builds exact history")
Y2 = 252
g.box(X[3], Y2, cw, 84, "S3", ["every raw event, 13 months"], "store", 12.5)
g.box(X[4], Y2, cw, 84, "Nightly Go job", ["recount yesterday exactly"], "batch", 12.5)
g.box(X[5], Y2, cw, 84, "DynamoDB", ["one count per campaign/day"], "store", 12.5)
g.arrow([(X[3] + cw/2, Y1 + 84), (X[3] + cw/2, Y2)], "archive", lx=X[3] + cw/2 + 8, ly=Y1 + 114, lanchor="start")
g.arrow([(X[3] + cw, Y2 + 42), (X[4], Y2 + 42)])
g.arrow([(X[4] + cw, Y2 + 42), (X[5], Y2 + 42)])

g.lane(14, 380, W - 28, 150, "3 · Marketers read insights")
Y3 = 418
g.box(X[0], Y3, cw, 84, "Marketer / retailer", ["calls the API"], "user", 12.5)
g.box(X[1], Y3, cw, 84, "Gateway", ["JWT check, rate limit"], "edge", 12.5)
g.box(X[2], Y3, cw, 84, "Insights API (Go)", ["cache + route by date"], "serve", 12.5)
g.arrow([(X[0] + cw, Y3 + 42), (X[1], Y3 + 42)])
g.arrow([(X[1] + cw, Y3 + 42), (X[2], Y3 + 42)])
# API -> DynamoDB (older) and Redis (recent)
g.arrow([(X[2] + cw, Y3 + 56), (X[5] + 70, Y3 + 56), (X[5] + 70, Y2 + 84)], "older than 48 h", lx=X[3] + 40, ly=Y3 + 50, lanchor="start")
g.arrow([(X[2] + cw, Y3 + 28), (R + 40, Y3 + 28), (R + 40, Y1 + 42), (R, Y1 + 42)], "last 48 h", lx=X[3] + 40, ly=Y3 + 22, lanchor="start")

g.text(24, 570, "Workers give fast numbers for the last 48 hours. The nightly job recounts each day from raw events (catching late events, bots, duplicates)", 12.5, "#334155")
g.text(24, 590, "and writes the exact daily count that history queries read.", 12.5, "#334155")
g.save("../hld.svg")
