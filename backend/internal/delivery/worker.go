package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
	"github.com/ranjitharani/webhook-delivery-platform/internal/queue"
	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

type Worker struct {
	store       *store.Store
	consumer    queue.Consumer
	httpClient  *http.Client
	driver      string
	concurrency int
}

func NewWorker(st *store.Store, consumer queue.Consumer, timeout time.Duration, driver string, concurrency int) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{
		store:    st,
		consumer: consumer,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		driver:      driver,
		concurrency: concurrency,
	}
}

func (w *Worker) Run(ctx context.Context) {
	jobs := make(chan uuid.UUID, w.concurrency*2)

	// Single consumer loop — kafka-go Reader is not safe for concurrent FetchMessage.
	go func() {
		defer close(jobs)
		for {
			eventID, err := w.consumer.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("consumer next error: %v", err)
				time.Sleep(time.Second)
				continue
			}
			select {
			case <-ctx.Done():
				return
			case jobs <- eventID:
			}
		}
	}()

	for i := 0; i < w.concurrency; i++ {
		go func(id int) {
			for eventID := range jobs {
				if err := w.process(ctx, eventID); err != nil {
					log.Printf("worker-%d process %s: %v", id, eventID, err)
				}
			}
		}(i)
	}

	<-ctx.Done()
}

func (w *Worker) process(ctx context.Context, eventID uuid.UUID) error {
	ev, err := w.store.GetEventForDelivery(ctx, eventID)
	if err != nil {
		return err
	}

	// Skip terminal states (idempotent consumer).
	if ev.Status == models.StatusDelivered || ev.Status == models.StatusDeadLetter {
		return nil
	}

	// For Kafka path, claim the event before delivering.
	if w.driver == "kafka" {
		if ev.Status == models.StatusRetrying && ev.NextAttemptAt.After(time.Now()) {
			// Not due yet; retry scheduler will republish later.
			return nil
		}
		if err := w.store.MarkProcessing(ctx, eventID); err != nil {
			if err == store.ErrConflict {
				return nil
			}
			return err
		}
		ev, err = w.store.GetEventForDelivery(ctx, eventID)
		if err != nil {
			return err
		}
	}

	result := w.deliver(ctx, ev)
	return w.store.RecordAttempt(ctx, ev, result)
}

func (w *Worker) deliver(ctx context.Context, ev *models.Event) store.AttemptResult {
	envelope := map[string]any{
		"id":              ev.ID.String(),
		"type":            ev.EventType,
		"idempotency_key": ev.IdempotencyKey,
		"created_at":      ev.CreatedAt.UTC().Format(time.RFC3339),
		"data":            json.RawMessage(ev.Payload),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return store.AttemptResult{Success: false, Error: err.Error()}
	}

	ts, sig := Sign(ev.EndpointSecret, body, time.Now().UTC())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ev.EndpointURL, bytes.NewReader(body))
	if err != nil {
		return store.AttemptResult{Success: false, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Id", ev.ID.String())
	req.Header.Set("X-Webhook-Timestamp", ts)
	req.Header.Set("X-Webhook-Signature", SignatureHeader(sig))
	req.Header.Set("X-Idempotency-Key", ev.IdempotencyKey)

	start := time.Now()
	resp, err := w.httpClient.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return store.AttemptResult{Success: false, LatencyMS: latency, Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	errMsg := ""
	if !success {
		errMsg = fmt.Sprintf("upstream status %d", resp.StatusCode)
	}
	return store.AttemptResult{
		Success:    success,
		HTTPStatus: resp.StatusCode,
		Body:       string(respBody),
		LatencyMS:  latency,
		Error:      errMsg,
	}
}
