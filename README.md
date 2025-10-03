# kafka partition skew example

This project demonstrates a **real-world partition skew issue** in Kafka and how to fix it using **key sharding**.  
It was inspired by a production issue where a **VIP player** consistently overloaded a single partition, causing **consumer lag** while other partitions stayed idle.

Read the full RCA: [Root Cause Analysis Report](./RCA.md)  

---

## problem

- **Symptom in production**: sustained lag on one partition of the `game-events` topic.
- **Cause**: Kafka assigns partitions based on the record key hash.  
  A single hot key (`player-000001`) always mapped to the same partition, creating a bottleneck.
- **Impact**: one consumer in the group lagged behind while others were idle, delaying downstream analytics.

---

## solution

The fix was to **shard the hot key** across multiple partitions:

```go
shard := rand.Intn(16)
key := fmt.Sprintf("%s#shard=%d", playerID, shard)
````

This keeps events logically tied to the player but distributes load more evenly across partitions.
Lag dropped and throughput became balanced.

---

## project structure

```
.
├── cmd
│   ├── consumer            # Kafka consumer (shows lag and skew)
│   ├── producer            # Producer (creates skew with hot key)
│   └── producer_balanced   # Producer with sharding (fixes skew)
├── docker-compose.yaml     # Zookeeper + Kafka setup
├── Makefile                # Helper commands
└── README.md               # This file
```

---

## prerequisites

* [Go 1.21+](https://go.dev/dl/)
* [Docker](https://docs.docker.com/get-docker/) + [Docker Compose](https://docs.docker.com/compose/)

---

## usage

### 1. Start Kafka + ZooKeeper

```bash
make start-kafka
```

This will spin up ZooKeeper + Kafka, wait for readiness, and create the `game-events` topic with 6 partitions.

### 2. Run the skewed producer

```bash
make run-producer
```

Simulates 95% of traffic from `player-000001`, causing partition skew.

### 3. Run the consumer

```bash
make run-consumer
```

Processes events and shows which partition is being overloaded.

Check lag:

```bash
make lag
```

### 4. Run the balanced producer (with sharding)

```bash
make run-balanced-producer
```

Now the VIP player’s events are spread across shards, balancing the load.

---

## sample output

### Skewed Producer

* Partition 2 overloaded with VIP traffic

```
partition=2 offset=500 key=player-000001
partition=2 offset=2000 key=player-000001
partition=2 offset=5500 key=player-000001
```

Consumer group lag:

```
partition=2 lag=250
others lag=0–3
```

### Balanced Producer

* VIP player spread across multiple partitions

```
partition=1 key=player-000001#shard=14
partition=5 key=player-000001#shard=9
partition=0 key=player-000001#shard=11
```

Consumer group lag:

```
partition=0 lag=28
partition=1 lag=26
partition=2 lag=38
partition=3 lag=35
partition=4 lag=77
partition=5 lag=89
```

---

## Learnings

* Always monitor partition-level metrics (`lag`, `offsets`, consumer balance).
* Anticipate **hot keys** and use **sharding strategies** to distribute load.
* Simple fixes like sharding can dramatically improve system balance.


