package outbox

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
	"github.com/ranjitharani/webhook-delivery-platform/internal/queue"
	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

type Relay struct {
	store     *store.Store
	publisher queue.Publisher
	driver    string
}

func NewRelay(st *store.Store, publisher queue.Publisher, driver string) *Relay {
	return &Relay{store: st, publisher: publisher, driver: driver}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.flushOutbox(ctx); err != nil {
				log.Printf("outbox relay error: %v", err)
			}
			if r.driver == "kafka" {
				if err := r.publishDueRetries(ctx); err != nil {
					log.Printf("retry publish error: %v", err)
				}
			}
		}
	}
}

func (r *Relay) flushOutbox(ctx context.Context) error {
	ids, payloads, err := r.store.ClaimOutbox(ctx, 50)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	// Postgres workers poll the events table directly.
	if r.driver == "postgres" {
		return nil
	}

	for _, payload := range payloads {
		var job models.DeliveryJob
		if err := json.Unmarshal(payload, &job); err != nil {
			log.Printf("outbox bad payload: %v", err)
			continue
		}
		if err := r.publisher.Publish(ctx, job.EventID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Relay) publishDueRetries(ctx context.Context) error {
	ids, err := r.store.PromoteDueRetries(ctx, 50)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.publisher.Publish(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
