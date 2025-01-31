package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
)

// BuildTag is the githash of this build.
// It is changed with build tags in the Makefile.
var BuildTag = "dev"

func main() {
	fmt.Printf("Veritone Engine Toolkit (%s)\n", BuildTag)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		cancel()
		signal.Stop(c)
	}()
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
			return
		}
	}()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	eng := NewEngine()
	eng.logDebug("engine: running")
	defer eng.logDebug("engine: stopped")
	skipKafka := false
	isTraining, err := isTrainingTask()
	if err != nil {
		eng.logDebug("assuming processing task because isTrainingTask error:", err)
	}
	if isTraining {
		skipKafka = true
	}
	if os.Getenv("VERITONE_TESTMODE") == "true" {
		eng.logDebug("WARNING: Test mode (remove VERITONE_TESTMODE before putting into production)")
		skipKafka = true
		eng.testMode = true
	}
	if eng.Config.SelfDriving.SelfDrivingMode {
		eng.logDebug("Running in file system mode (VERITONE_SELFDRIVING=true)")
		skipKafka = true
	}
	if !skipKafka {
		eng.logDebug("brokers:", eng.Config.Kafka.Brokers)
		eng.logDebug("consumer group:", eng.Config.Kafka.ConsumerGroup)
		eng.logDebug("input topic:", eng.Config.Kafka.InputTopic)
		eng.logDebug("chunk topic:", eng.Config.Kafka.ChunkTopic)
		var err error
		var cleanup func()
		eng.consumer, cleanup, err = newKafkaConsumer(eng.Config.Kafka.Brokers, eng.Config.Kafka.ConsumerGroup, eng.Config.Kafka.InputTopic)
		if err != nil {
			return errors.Wrap(err, "kafka consumer")
		}
		defer cleanup()
		eng.producer, err = newKafkaProducer(eng.Config.Kafka.Brokers)
		if err != nil {
			return errors.Wrap(err, "kafka producer")
		}
		// use the same producer for events
		eng.eventProducer = eng.producer
	} else {
		eng.logDebug("skipping kafka setup")
	}
	if err := eng.Run(ctx); err != nil {
		return err
	}
	return nil
}
