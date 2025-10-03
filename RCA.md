# Root Cause Analysis Report

**Author:** Tiago Melo (tiagoharris@gmail.com)

**Repository:** [github.com/tiagomelo/kafka-partition-skew-example](https://github.com/tiagomelo/kafka-partition-skew-example)

---

## 1. Issue Overview

In our production game analytics pipeline, I noticed **sustained consumer lag on a single Kafka partition** while others remained idle.
This was traced back to a **partition skew issue**: traffic from a VIP player was consistently routed to a single partition, creating a hotspot.

To better understand and demonstrate the problem, I created a reproducible simulation in [kafka-partition-skew-example](https://github.com/tiagomelo/kafka-partition-skew-example).
The project shows both the **failure mode** and a **practical fix**.

---

## 2. System Context

* **Domain**: Real-time game analytics (bets, hand completions, wallet transactions).
* **Architecture**: Event-driven, Kafka-based pipeline.
* **Symptoms in Production**:

  * Consumer group lag consistently growing on one partition.
  * Downstream processing delayed for VIP-related events.

---

## 3. Investigation (Simulation Findings)

### Evidence of Skew (Before Fix)

Producer with 95% of traffic for `player-000001`:

```json
{"msg":"producer started","brokers":"localhost:29092","topic":"game-events","rate":400,"skewPercent":95}
```

Consumer logs (partition 2 overloaded):

```json
{"partition":2,"offset":500,"key":"player-000001"}
{"partition":2,"offset":2000,"key":"player-000001"}
{"partition":2,"offset":5500,"key":"player-000001"}
```

Consumer group lag:

```
partition=2 lag=250
others lag=0–3
```

Clear imbalance: one consumer tied to partition 2 was consistently behind.

---

### Root Cause

* Kafka’s partitioner hashes the **key** (`player-000001`).
* Since all VIP traffic used the same key, it always landed in **partition 2**.
* With range assignment, that partition was bound to one consumer, creating a bottleneck.

---

## 4. Corrective Action

### Fix: **Key Sharding**

Updated producer logic to shard hot keys:

```go
shard := rand.Intn(16)
key := fmt.Sprintf("%s#shard=%d", playerID, shard)
```

Now, the VIP traffic is spread across multiple partitions while preserving per-shard ordering.

---

### Evidence After Fix

Balanced producer logs:

```json
{"msg":"balanced producer started","brokers":"localhost:29092","topic":"game-events","rate":400,"shards":16}
```

Consumer logs (VIP spread across partitions):

```json
{"partition":1,"key":"player-000001#shard=14"}
{"partition":5,"key":"player-000001#shard=9"}
{"partition":0,"key":"player-000001#shard=11"}
```

Consumer group lag (balanced):

```
partition=0 lag=28
partition=1 lag=26
partition=2 lag=38
partition=3 lag=35
partition=4 lag=77
partition=5 lag=89
```

Lag distributed across partitions, no single bottleneck.

---

## 5. Recommendations

1. **Design for hot keys**: anticipate skew from VIPs or popular items.
2. **Sharding strategy**: add a shard suffix to distribute load while retaining logical grouping.
3. **Monitoring**: build partition-level lag dashboards in Grafana/Prometheus.
4. **Capacity planning**: provision partitions and consumers to absorb surges.

---

## 6. Conclusion

The production issue was caused by **hot key concentration**, leading to partition skew and consumer lag.
The root cause was successfully reproduced in a controlled environment, and the sharding strategy demonstrated in the simulation repo resolved the problem.

Repo: [github.com/tiagomelo/kafka-partition-skew-example](https://github.com/tiagomelo/kafka-partition-skew-example)
