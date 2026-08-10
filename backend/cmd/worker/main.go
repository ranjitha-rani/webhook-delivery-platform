package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ranjitharani/webhook-delivery-platform/internal/config"
	"github.com/ranjitharani/webhook-delivery-platform/internal/db"
	"github.com/ranjitharani/webhook-delivery-platform/internal/delivery"
	"github.com/ranjitharani/webhook-delivery-platform/internal/queue"
	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)

	var consumer queue.Consumer
	switch cfg.QueueDriver {
	case "kafka":
		consumer = queue.NewKafkaConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, "webhook-workers")
		log.Printf("worker queue driver: kafka")
	default:
		consumer = queue.NewPostgresConsumer(st)
		cfg.QueueDriver = "postgres"
		log.Printf("worker queue driver: postgres")
	}
	defer consumer.Close()

	worker := delivery.NewWorker(st, consumer, cfg.HTTPTimeout, cfg.QueueDriver, cfg.WorkerConcurrency)
	log.Printf("worker started concurrency=%d", cfg.WorkerConcurrency)
	worker.Run(ctx)
	log.Println("worker stopped")
	os.Exit(0)
}
