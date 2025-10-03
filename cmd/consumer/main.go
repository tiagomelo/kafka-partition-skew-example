// Copyright (c) 2025 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package main

import (
	"context"
	"log/slog"
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
	Group        string `short:"g" long:"group" description:"Kafka consumer group" required:"true"`
	WorkMs       int    `short:"w" long:"work-ms" description:"Milliseconds to simulate work per message" default:"2"`
}

// handler represents a Sarama consumer group handler.
type handler struct {
	work time.Duration
	log  *slog.Logger
}

// Setup is run at the beginning of a new session, before ConsumeClaim.
// This is a no-op.
func (h handler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited.
// This is a no-op.
func (h handler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
// This is called within a goroutine, so spawning additional goroutines to
// consume messages is not necessary. Messages must be marked as processed
// by calling the MarkMessage method of the ConsumerGroupSession.
func (h handler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	p := claim.Partition()
	for msg := range claim.Messages() {
		// simulate constant work per message.
		time.Sleep(h.work)

		// log every N offsets to visualize partition skew.
		if msg.Offset%500 == 0 {
			h.log.Info("message",
				slog.Int("partition", int(p)),
				slog.Int64("offset", msg.Offset),
				slog.String("key", string(msg.Key)),
			)
		}
		// mark message as processed.
		sess.MarkMessage(msg, "")
	}
	return nil
}

// run starts the consumer.
func run(kafkaBrokers, topic, group string, workMs int, log *slog.Logger) error {
	ctx := context.Background()
	defer log.InfoContext(ctx, "completed")

	brokers := strings.Split(kafkaBrokers, ",")

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V3_6_0_0
	// range strategy makes skew obvious (same partition stays with same member).
	cfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRange()
	cfg.Consumer.Return.Errors = true

	client, err := sarama.NewConsumerGroup(brokers, group, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create kafka consumer group")
	}
	defer client.Close()

	log.InfoContext(ctx, "consumer started",
		slog.String("brokers", kafkaBrokers),
		slog.String("group", group),
		slog.String("topic", topic),
	)

	// shutdown and error channels.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// capture errors when consuming messages.
	consumeErrors := make(chan error, 1)

	go func() {
		for {
			consumeErrors <- client.Consume(ctx, []string{topic}, handler{work: time.Duration(workMs) * time.Millisecond, log: log})
		}
	}()

	// wait for shutdown or error.
	select {
	case err := <-consumeErrors:
		log.ErrorContext(ctx, "consume error", slog.Any("err", err))

	case sig := <-shutdown:
		log.InfoContext(ctx, "shutdown started", slog.Any("sig", sig))

		// graceful shutdown with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Close(); err != nil {
			log.ErrorContext(ctx, "failed to close consumer", slog.Any("err", err))
		}
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
	if err := run(opts.KafkaBrokers, opts.Topic, opts.Group, opts.WorkMs, log); err != nil {
		log.Error("error", slog.Any("err", err))
		os.Exit(1)
	}
}
