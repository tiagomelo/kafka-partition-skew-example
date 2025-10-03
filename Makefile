SHELL := /bin/bash

# ==============================================================================
# Kafka configuration variables

PARTITIONS ?= 6
TOPIC ?= game-events
GROUP ?= games-analytics

# ==============================================================================
# Help

.PHONY: help
## help: shows this help message
help:
	@echo "Usage: make [target]\n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

# ==============================================================================
# Kafka

.PHONY: start-kafka
## start-kafka: starts ZooKeeper + Kafka and creates topic
start-kafka:
	@docker compose up -d
	@echo "Waiting for Kafka to start..."
	@until docker compose exec -T kafka kafka-topics --bootstrap-server kafka:9092 --list >/dev/null 2>&1; do \
		echo "Kafka not ready, sleeping for 5 seconds..."; \
		sleep 5; \
	done
	@echo "Kafka is up and running."
	@docker compose exec -T kafka kafka-topics --create --topic $(TOPIC) \
		--bootstrap-server kafka:9092 --if-not-exists --replication-factor 1 --partitions $(PARTITIONS)
	@echo "Topic $(TOPIC) ready with $(PARTITIONS) partitions."

.PHONY: stop-kafka
## stop-kafka: stops ZooKeeper + Kafka and removes data
stop-kafka:
	@docker compose down -v
	@echo "Kafka and ZooKeeper stopped."

.PHONY: partitions
## partitions: shows partition assignment for the specified topic
partitions:
	@docker compose exec -T kafka kafka-topics --describe --topic $(TOPIC) --bootstrap-server kafka:9092

.PHONY: lag
## lag: shows the consumer group lag for the specified group
lag:
	@docker compose exec -T kafka kafka-consumer-groups --bootstrap-server kafka:9092 --describe --group $(GROUP) || true

.PHONY: topics
## topics: list all topics
topics:
	@docker compose exec -T kafka kafka-topics --list --bootstrap-server kafka:9092

# ==============================================================================
# Kafka consumer

.PHONY: run-consumer
## run-consumer: runs the Kafka consumer
run-consumer:
	@go run cmd/consumer/main.go --topic $(TOPIC) --group $(GROUP)

# ==============================================================================
# Kafka producer

.PHONY: run-producer
## run-producer: runs the Kafka producer
run-producer:
	@go run cmd/producer/main.go --topic $(TOPIC)

# ==============================================================================
# Kafka balanced producer

.PHONY: run-balanced-producer
## run-balanced-producer: runs the Kafka balanced producer
run-balanced-producer:
	@go run cmd/producer_balanced/main.go --topic $(TOPIC)