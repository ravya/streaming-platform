---
title: "Assumptions and Sizing"
subtitle: "Why each number in the design proposal was chosen"
---

# 1. Read this first

The case study gives no traffic numbers. Every number in the design proposal is my estimate, used to size the system. None of them come from real data. If you have real figures, replace the inputs in section 4 and the sizing follows from the formulas in section 5.

The architecture does not depend on these numbers. If an estimate is wrong, we add more Redis shards, more worker pods or more partitions. Nothing needs redesigning.

# 2. Why 20% of events feed insights

**What is assumed.** Out of every 100 events the platform receives, about 20 are impressions, clicks or add-to-cart events. These are the only events the three APIs count. The other 80 (product views, searches, page loads, scrolls) are stored but not counted.

**Why I picked a minority.** In one shopping session a customer searches, scrolls listings, opens several products, and maybe adds one to the cart. Most of those actions are not ad events. Ads only appear in a limited number of sponsored slots per page, and few of them get clicked.

**Why not much lower.** Every sponsored slot shown is logged as an impression, even if nobody clicks. A listing page with several sponsored products creates several impression events per page view. So ad events can be a large share, not a tiny one.

**Why 20%.** It sits in the lower-middle of a range I think is plausible (10% to 50%). It is a guess. I chose it to size the system, not because I measured it.

**What changes if it is wrong.** Only the size of the ad-event path. Everything is linear.

| Share of events that are ad events | 10% | 20% (used) | 50% |
|---|---|---|---|
| Ad events/s, average | 10K | 20K | 50K |
| Ad events/s, sale burst | 100K | 200K | 500K |
| Redis hot data (48 h) | 39 GB | 78 GB | 194 GB |
| Redis hot nodes (primary + replica) | 4 | 8 | 20 |
| Redis dedup/state data at burst | 22 GB | 43 GB | 108 GB |
| Worker pods at burst | 7 | 14 | 34 |
| Kafka `events.ads` minimum partitions | 5 | 10 | 25 |
| S3 ad events, 13 months | 34 TB | 68 TB | 171 TB |

**What to check first with real data.** The ad-event share. It drives the Redis bill, which is the largest cost in this design.

# 3. Traffic volume

| Input | Value | Reasoning |
|---|---|---|
| Average events | 100K/s (8.6 B/day) | A large retailer can have tens of millions of daily shoppers generating a few hundred events each, which is several billion events a day. Most of the 20 launch retailers are smaller. Total of 8 to 9 billion a day is plausible. |
| Normal peak | 300K/s (3x average) | Evening hours are typically two to three times the daily average. |
| Sale-event burst | 1M/s (10x average) for up to 2 hours | Big sale events can reach several times the normal peak. I took 10x average as the design target so we have room. |
| Event size | 500 bytes | A small JSON/Protobuf record with IDs, type and timestamps. |
| Batch size | 50 events per request | SDKs usually batch to save battery and connections. |

# 4. Other inputs

| Input | Value | Reasoning |
|---|---|---|
| Retailers | 20 now, 100+ in two years | A starting guess for a platform serving a retail network. Affects config and quota management, not volume (volume is set above). |
| Active campaigns | 200K, with 3 metrics each | Roughly 10,000 campaigns per retailer at launch. Gives about 600K daily result rows. |
| Unique-customer ratio per bucket | 50% | Within a 5-minute window the same customer often shows up several times (several impressions, a click). Half as many distinct customers as events is a middle estimate. |
| Redis bytes per set member | about 45 | Redis sets of short strings take roughly 40 to 60 bytes per member including overhead. Must be confirmed with a test. |
| Dedup window | 1 hour | Duplicates come from SDK retries, which usually happen within minutes. One hour is generous. |
| Attribution window | 30 minutes | A common default for click-to-cart attribution. Configurable per tenant. |
| API users | 5,000 dashboard users, 6 queries each, refresh every 30 s | Gives about 1,000 requests/s normally and about 10,000/s during a sale (10x). Most queries repeat, so the cache absorbs a lot. |
| Compression | Parquet is about 5x smaller than JSON | Typical for columnar files with repeated values. |

# 5. Capacity at the numbers used (20%)

| Component | Calculation | Result |
|---|---|---|
| Collector | 1M events/s ÷ 50 per request = 20K requests/s; about 2K requests/s per pod | 10 pods at burst, scale to 40 |
| Kafka `events.ads` | 200K/s × 500 B = 100 MB/s; about 10 MB/s per partition | 10 minimum; plan 48 (headroom and worker parallelism) |
| Kafka `events.other` | 800K/s × 500 B = 400 MB/s | 40 minimum; plan 128 |
| Kafka brokers | About 500 MB/s in, three copies on disk | 9 brokers (3 per zone) |
| Stream workers | 200K ad events/s ÷ about 15K per pod | 14 pods at burst, scale up to 48 |
| Redis hot | 20K/s × 48 h ≈ 3.5 B events; 50% unique ≈ 1.7 B members × 45 B | about 78 GB, 4 primaries + 4 replicas |
| Redis state | 200K/s × 1 h dedup keys × 60 B | about 43 GB at burst, 3 primaries + replicas |
| DynamoDB | 200K × 3 metrics × 365 days × 3 years × about 100 B | about 660 M rows, 66 GB |
| S3 | Ad events: 20K/s × 86,400 × 500 B ÷ 5 | about 0.17 TB/day, 68 TB for 13 months; other events about 21 TB for 30 days |
| Insights API | 10K requests/s ÷ about 2K per pod | 5 to 6 pods at peak, autoscale 3 to 60 |

Per-pod and per-partition throughput (2K requests/s, 15K events/s, 10 MB/s) are planning figures. A load test in phase 1 replaces them with measured values.

# 6. Using real data

1. Count events by type for one week from an existing retailer. This gives the real ad-event share, the peak factors and the event size.
2. Run a sample of real customer IDs through a 5-minute window to get the real unique ratio.
3. Load test one worker pod, one collector pod and one Redis node to replace the per-pod figures.
4. Re-run the numbers in section 5 with the measured inputs and update the proposal.
