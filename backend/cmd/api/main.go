package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ranjitharani/webhook-delivery-platform/internal/api"
	"github.com/ranjitharani/webhook-delivery-platform/internal/config"
	"github.com/ranjitharani/webhook-delivery-platform/internal/db"
	"github.com/ranjitharani/webhook-delivery-platform/internal/outbox"
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

	var publisher queue.Publisher
	switch cfg.QueueDriver {
	case "kafka":
		publisher = queue.NewKafkaPublisher(cfg.KafkaBrokers, cfg.KafkaTopic)
		log.Printf("queue driver: kafka (%v topic=%s)", cfg.KafkaBrokers, cfg.KafkaTopic)
	default:
		publisher = queue.NewPostgresPublisher()
		cfg.QueueDriver = "postgres"
		log.Printf("queue driver: postgres")
	}
	defer publisher.Close()

	relay := outbox.NewRelay(st, publisher, cfg.QueueDriver)
	go relay.Run(ctx)

	server := api.NewServer(st, cfg.CORSOrigins, cfg.QueueDriver)
	httpServer := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("api listening on %s", cfg.APIAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Println("api stopped")
	os.Exit(0)
}
