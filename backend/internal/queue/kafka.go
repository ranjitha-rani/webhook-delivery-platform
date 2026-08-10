package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
)

type KafkaPublisher struct {
	writer *kafka.Writer
}

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			Async:        false,
		},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, eventID uuid.UUID) error {
	body, err := json.Marshal(models.DeliveryJob{EventID: eventID})
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(eventID.String()),
		Value: body,
		Time:  time.Now(),
	})
}

func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}

type KafkaConsumer struct {
	reader *kafka.Reader
}

func NewKafkaConsumer(brokers []string, topic, group string) *KafkaConsumer {
	return &KafkaConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        group,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
	}
}

func (c *KafkaConsumer) Next(ctx context.Context) (uuid.UUID, error) {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var job models.DeliveryJob
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		_ = c.reader.CommitMessages(ctx, msg)
		return uuid.Nil, fmt.Errorf("invalid kafka payload: %w", err)
	}
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		return uuid.Nil, err
	}
	return job.EventID, nil
}

func (c *KafkaConsumer) Close() error {
	return c.reader.Close()
}
