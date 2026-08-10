package queue

import (
	"context"

	"github.com/google/uuid"
)

type Publisher interface {
	Publish(ctx context.Context, eventID uuid.UUID) error
	Close() error
}

type Consumer interface {
	// Next blocks until a job is available or ctx is cancelled.
	Next(ctx context.Context) (uuid.UUID, error)
	Close() error
}
