// Copyright (c) 2025 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
)

// options represents the command line options.
type options struct {
	KafkaBrokers string `short:"b" long:"brokers" description:"Comma separated list of Kafka brokers" default:"localhost:29092"`
	Topic        string `short:"t" long:"topic" description:"Kafka topic to produce messages to" required:"true"`
	Rate         int    `short:"r" long:"rate" description:"Messages per second to produce" default:"400"`
	Shards       int    `short:"s" long:"shards" description:"Number of shards to spread hot keys across" default:"16"`
}

// gameEvent represents a game event message.
type gameEvent struct {
	ID        int64
	PlayerID  string
	GameID    string
	EventType string
	Amount    int
	TableID   string
	TS        int64
}

// run starts the balanced producer.
func run(kafkaBrokers, topic string, rate, shards int, log *slog.Logger) error {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	ctx := context.Background()
	defer log.InfoContext(ctx, "completed")

	brokers := strings.Split(kafkaBrokers, ",")
	cfg := sarama.NewConfig()
	// wait for only the local commit to succeed before responding.
	cfg.Producer.RequiredAcks = sarama.WaitForLocal
	// the producer will wait for all in-sync replicas to ack the message
	// before responding.
	asyncProducer, err := sarama.NewAsyncProducer(brokers, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create kafka producer")
	}
	defer asyncProducer.Close()

	log.InfoContext(ctx, "balanced producer started",
		slog.String("brokers", kafkaBrokers),
		slog.String("topic", topic),
		slog.Int("rate", rate),
		slog.Int("shards", shards),
	)

	// control the rate of message production.
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	// shutdown handling.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		var i int64
		for {
			<-ticker.C

			// VIP dominates traffic
			playerID := "player-000001"
			if r.Intn(100) >= 95 { // 5% non-VIP.
				playerID = fmt.Sprintf("player-%06d", r.Intn(200000))
			}

			// create a realistic event (bet placed / hand finished).
			ev := gameEvent{
				ID:        i,
				PlayerID:  playerID,
				GameID:    fmt.Sprintf("game-%d", r.Intn(50)),
				EventType: pick([]string{"bet_placed", "hand_finished", "chips_added"}, r),
				Amount:    10 + r.Intn(200),
				TableID:   fmt.Sprintf("table-%d", r.Intn(100)),
				TS:        time.Now().UnixMilli(),
			}

			// shard the VIP key.
			shard := r.Intn(shards)
			key := fmt.Sprintf("%s#shard=%d", ev.PlayerID, shard)

			// simple JSON encoding.
			// in production use something like protobuf or avro.
			val := fmt.Sprintf(
				`{"id":%d,"player_id":"%s","game_id":"%s","event_type":"%s","amount":%d,"table_id":"%s","ts":%d}`,
				ev.ID, ev.PlayerID, ev.GameID, ev.EventType, ev.Amount, ev.TableID, ev.TS,
			)

			msg := &sarama.ProducerMessage{
				Topic: topic,
				Key:   sarama.StringEncoder(key),
				Value: sarama.StringEncoder(val),
			}

			select {
			case asyncProducer.Input() <- msg: // send message to Kafka.
				i++
			default:
				// drop on backpressure.
			}
		}
	}()

	// wait for shutdown.
	sig := <-shutdown
	log.InfoContext(ctx, "shutdown started", slog.Any("sig", sig))

	// graceful shutdown with timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		asyncProducer.Close()
		close(done)
	}()

	select {
	case <-done:
		log.InfoContext(shutdownCtx, "producer closed cleanly")
	case <-shutdownCtx.Done():
		log.Error("producer close timed out", slog.Any("err", shutdownCtx.Err()))
	}

	return nil
}

func main() {
	var opts options
	parser := flags.NewParser(&opts, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(opts.KafkaBrokers, opts.Topic, opts.Rate, opts.Shards, log); err != nil {
		log.Error("error", slog.Any("err", err))
		os.Exit(1)
	}
}

// pick returns a random string from a slice.
func pick(ss []string, r *rand.Rand) string {
	return ss[r.Intn(len(ss))]
}
