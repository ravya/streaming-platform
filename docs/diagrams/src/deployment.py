from svgkit import SVG
W, H = 1560, 600
g = SVG(W, H, "Deployment")

g.box(329, 56, 430, 50, "Route 53 → CloudFront + WAF → ALB", [], "edge", 12.5)
g.lane(14, 122, 1060, 330, "Primary region (3 availability zones)")
for i, az in enumerate(["AZ a", "AZ b", "AZ c"]):
    x = 30 + i * 345
    g.lane(x, 150, 330, 290, az, "#FFFFFF", "#94A3B8", "#334155")
    g.box(x + 14, 178, 302, 74, "EKS nodes", ["collector, stream workers,", "Insights API, nightly job"], "ingest", 12)
    g.box(x + 14, 266, 302, 58, "Kafka broker", ["replication factor 3 across AZs"], "kafka", 12)
    g.box(x + 14, 338, 302, 58, "Redis shard", ["replica sits in another AZ"], "store", 12)
g.arrow([(544, 106), (544, 150)], "", "n")
g.box(30, 466, 330, 84, "S3", ["raw events, replicated to DR region"], "store", 12)
g.box(375, 466, 330, 84, "DynamoDB", ["daily counts + tenant config, global table"], "store", 12)
g.box(720, 466, 354, 84, "Monitoring", ["Prometheus, Grafana, Loki, OpenTelemetry"], "xc", 12)

g.lane(1096, 122, 450, 428, "DR region (standby)", "#FEF2F2", "#FCA5A5", "#991B1B")
g.box(1114, 160, 414, 66, "Kafka mirror", ["MirrorMaker 2 copies topics"], "kafka", 12)
g.box(1114, 240, 414, 66, "Data copies", ["S3 and DynamoDB replicate here"], "store", 12)
g.box(1114, 320, 414, 66, "Small EKS cluster", ["scaled up only during failover"], "ingest", 12)
g.box(1114, 400, 414, 66, "Route 53 failover", ["RPO 5 min, RTO 30 min"], "edge", 12)
g.save("../deployment.svg")
