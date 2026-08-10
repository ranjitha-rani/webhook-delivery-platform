package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDelivered  = "delivered"
	StatusRetrying   = "retrying"
	StatusDeadLetter = "dead_letter"
	StatusFailed     = "failed"
)

type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type Endpoint struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	EndpointID     uuid.UUID       `json:"endpoint_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	AttemptCount   int             `json:"attempt_count"`
	MaxAttempts    int             `json:"max_attempts"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	EndpointURL    string          `json:"endpoint_url,omitempty"`
	EndpointSecret string          `json:"-"`
	EndpointName   string          `json:"endpoint_name,omitempty"`
}

type DeliveryAttempt struct {
	ID            uuid.UUID `json:"id"`
	EventID       uuid.UUID `json:"event_id"`
	AttemptNumber int       `json:"attempt_number"`
	Status        string    `json:"status"`
	HTTPStatus    *int      `json:"http_status,omitempty"`
	ResponseBody  *string   `json:"response_body,omitempty"`
	LatencyMS     *int      `json:"latency_ms,omitempty"`
	Error         *string   `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type DeliveryJob struct {
	EventID uuid.UUID `json:"event_id"`
}

type Stats struct {
	Pending    int `json:"pending"`
	Retrying   int `json:"retrying"`
	Delivered  int `json:"delivered"`
	DeadLetter int `json:"dead_letter"`
	Processing int `json:"processing"`
	Total      int `json:"total"`
}

type EndpointHealth struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	IsActive        bool      `json:"is_active"`
	EventsTotal     int       `json:"events_total"`
	Delivered       int       `json:"delivered"`
	DeadLetter      int       `json:"dead_letter"`
	Retrying        int       `json:"retrying"`
	SuccessAttempts int       `json:"success_attempts"`
	FailedAttempts  int       `json:"failed_attempts"`
	AvgLatencyMS    float64   `json:"avg_latency_ms"`
}
