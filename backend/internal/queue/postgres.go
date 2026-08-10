package queue

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

// PostgresPublisher is a no-op for publish: jobs are claimed directly from events table.
type PostgresPublisher struct{}

func NewPostgresPublisher() *PostgresPublisher { return &PostgresPublisher{} }

func (p *PostgresPublisher) Publish(ctx context.Context, eventID uuid.UUID) error {
	return nil
}

func (p *PostgresPublisher) Close() error { return nil }

type PostgresConsumer struct {
	store *store.Store
}

func NewPostgresConsumer(st *store.Store) *PostgresConsumer {
	return &PostgresConsumer{store: st}
}

func (c *PostgresConsumer) Next(ctx context.Context) (uuid.UUID, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		events, err := c.store.ClaimDueEvents(ctx, 1)
		if err != nil {
			return uuid.Nil, err
		}
		if len(events) > 0 {
			return events[0].ID, nil
		}

		select {
		case <-ctx.Done():
			return uuid.Nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *PostgresConsumer) Close() error { return nil }
