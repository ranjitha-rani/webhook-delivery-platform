package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) GetTenantByAPIKey(ctx context.Context, apiKey string) (*models.Tenant, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, api_key, created_at
		FROM tenants WHERE api_key = $1
	`, apiKey)

	var t models.Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.APIKey, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateEndpoint(ctx context.Context, tenantID uuid.UUID, name, url, secret string) (*models.Endpoint, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO endpoints (tenant_id, name, url, secret)
		VALUES ($1, $2, $3, $4)
		RETURNING id, tenant_id, name, url, secret, is_active, created_at
	`, tenantID, name, url, secret)

	var e models.Endpoint
	if err := row.Scan(&e.ID, &e.TenantID, &e.Name, &e.URL, &e.Secret, &e.IsActive, &e.CreatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) ListEndpoints(ctx context.Context, tenantID uuid.UUID) ([]models.Endpoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, url, secret, is_active, created_at
		FROM endpoints
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Endpoint
	for rows.Next() {
		var e models.Endpoint
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Name, &e.URL, &e.Secret, &e.IsActive, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) SetEndpointActive(ctx context.Context, tenantID, endpointID uuid.UUID, active bool) (*models.Endpoint, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE endpoints
		SET is_active = $3
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, url, secret, is_active, created_at
	`, endpointID, tenantID, active)

	var e models.Endpoint
	if err := row.Scan(&e.ID, &e.TenantID, &e.Name, &e.URL, &e.Secret, &e.IsActive, &e.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (s *Store) ListEndpointHealth(ctx context.Context, tenantID uuid.UUID) ([]models.EndpointHealth, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			ep.id,
			ep.name,
			ep.url,
			ep.is_active,
			COUNT(e.id) AS events_total,
			COUNT(e.id) FILTER (WHERE e.status = 'delivered') AS delivered,
			COUNT(e.id) FILTER (WHERE e.status = 'dead_letter') AS dead_letter,
			COUNT(e.id) FILTER (WHERE e.status = 'retrying') AS retrying,
			COALESCE((
				SELECT COUNT(*) FROM delivery_attempts da
				JOIN events ev ON ev.id = da.event_id
				WHERE ev.endpoint_id = ep.id AND da.status = 'delivered'
			), 0) AS success_attempts,
			COALESCE((
				SELECT COUNT(*) FROM delivery_attempts da
				JOIN events ev ON ev.id = da.event_id
				WHERE ev.endpoint_id = ep.id AND da.status = 'failed'
			), 0) AS failed_attempts,
			COALESCE((
				SELECT AVG(da.latency_ms)::float8 FROM delivery_attempts da
				JOIN events ev ON ev.id = da.event_id
				WHERE ev.endpoint_id = ep.id AND da.latency_ms IS NOT NULL
			), 0) AS avg_latency_ms
		FROM endpoints ep
		LEFT JOIN events e ON e.endpoint_id = ep.id
		WHERE ep.tenant_id = $1
		GROUP BY ep.id
		ORDER BY ep.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EndpointHealth
	for rows.Next() {
		var h models.EndpointHealth
		if err := rows.Scan(
			&h.ID, &h.Name, &h.URL, &h.IsActive,
			&h.EventsTotal, &h.Delivered, &h.DeadLetter, &h.Retrying,
			&h.SuccessAttempts, &h.FailedAttempts, &h.AvgLatencyMS,
		); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetEndpoint(ctx context.Context, tenantID, endpointID uuid.UUID) (*models.Endpoint, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, url, secret, is_active, created_at
		FROM endpoints
		WHERE id = $1 AND tenant_id = $2
	`, endpointID, tenantID)

	var e models.Endpoint
	if err := row.Scan(&e.ID, &e.TenantID, &e.Name, &e.URL, &e.Secret, &e.IsActive, &e.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

type CreateEventInput struct {
	TenantID       uuid.UUID
	EndpointID     uuid.UUID
	EventType      string
	Payload        json.RawMessage
	IdempotencyKey string
}

func (s *Store) CreateEventWithOutbox(ctx context.Context, in CreateEventInput) (*models.Event, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	var endpointActive bool
	err = tx.QueryRow(ctx, `
		SELECT is_active FROM endpoints WHERE id = $1 AND tenant_id = $2
	`, in.EndpointID, in.TenantID).Scan(&endpointActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	if !endpointActive {
		return nil, false, fmt.Errorf("endpoint inactive")
	}

	var existing models.Event
	err = tx.QueryRow(ctx, `
		SELECT id, tenant_id, endpoint_id, event_type, payload, idempotency_key, status,
		       attempt_count, max_attempts, next_attempt_at, last_error, created_at, updated_at
		FROM events
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, in.TenantID, in.IdempotencyKey).Scan(
		&existing.ID, &existing.TenantID, &existing.EndpointID, &existing.EventType, &existing.Payload,
		&existing.IdempotencyKey, &existing.Status, &existing.AttemptCount, &existing.MaxAttempts,
		&existing.NextAttemptAt, &existing.LastError, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return &existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}

	var ev models.Event
	err = tx.QueryRow(ctx, `
		INSERT INTO events (tenant_id, endpoint_id, event_type, payload, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, endpoint_id, event_type, payload, idempotency_key, status,
		          attempt_count, max_attempts, next_attempt_at, last_error, created_at, updated_at
	`, in.TenantID, in.EndpointID, in.EventType, in.Payload, in.IdempotencyKey).Scan(
		&ev.ID, &ev.TenantID, &ev.EndpointID, &ev.EventType, &ev.Payload, &ev.IdempotencyKey,
		&ev.Status, &ev.AttemptCount, &ev.MaxAttempts, &ev.NextAttemptAt, &ev.LastError,
		&ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		return nil, false, err
	}

	job, _ := json.Marshal(models.DeliveryJob{EventID: ev.ID})
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (event_id, payload) VALUES ($1, $2)
	`, ev.ID, job)
	if err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &ev, false, nil
}

func (s *Store) ListEvents(ctx context.Context, tenantID uuid.UUID, status string, limit int) ([]models.Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
		SELECT e.id, e.tenant_id, e.endpoint_id, e.event_type, e.payload, e.idempotency_key, e.status,
		       e.attempt_count, e.max_attempts, e.next_attempt_at, e.last_error, e.created_at, e.updated_at,
		       ep.url, ep.name
		FROM events e
		JOIN endpoints ep ON ep.id = e.endpoint_id
		WHERE e.tenant_id = $1
	`
	args := []any{tenantID}
	if status != "" {
		query += ` AND e.status = $2`
		args = append(args, status)
		query += ` ORDER BY e.created_at DESC LIMIT $3`
		args = append(args, limit)
	} else {
		query += ` ORDER BY e.created_at DESC LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.EndpointID, &e.EventType, &e.Payload, &e.IdempotencyKey, &e.Status,
			&e.AttemptCount, &e.MaxAttempts, &e.NextAttemptAt, &e.LastError, &e.CreatedAt, &e.UpdatedAt,
			&e.EndpointURL, &e.EndpointName,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetEvent(ctx context.Context, tenantID, eventID uuid.UUID) (*models.Event, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT e.id, e.tenant_id, e.endpoint_id, e.event_type, e.payload, e.idempotency_key, e.status,
		       e.attempt_count, e.max_attempts, e.next_attempt_at, e.last_error, e.created_at, e.updated_at,
		       ep.url, ep.secret, ep.name
		FROM events e
		JOIN endpoints ep ON ep.id = e.endpoint_id
		WHERE e.id = $1 AND e.tenant_id = $2
	`, eventID, tenantID)

	var e models.Event
	if err := row.Scan(
		&e.ID, &e.TenantID, &e.EndpointID, &e.EventType, &e.Payload, &e.IdempotencyKey, &e.Status,
		&e.AttemptCount, &e.MaxAttempts, &e.NextAttemptAt, &e.LastError, &e.CreatedAt, &e.UpdatedAt,
		&e.EndpointURL, &e.EndpointSecret, &e.EndpointName,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (s *Store) ListAttempts(ctx context.Context, eventID uuid.UUID) ([]models.DeliveryAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_id, attempt_number, status, http_status, response_body, latency_ms, error, created_at
		FROM delivery_attempts
		WHERE event_id = $1
		ORDER BY attempt_number ASC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.DeliveryAttempt
	for rows.Next() {
		var a models.DeliveryAttempt
		if err := rows.Scan(
			&a.ID, &a.EventID, &a.AttemptNumber, &a.Status, &a.HTTPStatus, &a.ResponseBody,
			&a.LatencyMS, &a.Error, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Stats(ctx context.Context, tenantID uuid.UUID) (*models.Stats, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'retrying') AS retrying,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'dead_letter') AS dead_letter,
			COUNT(*) FILTER (WHERE status = 'processing') AS processing,
			COUNT(*) AS total
		FROM events
		WHERE tenant_id = $1
	`, tenantID)

	var st models.Stats
	if err := row.Scan(&st.Pending, &st.Retrying, &st.Delivered, &st.DeadLetter, &st.Processing, &st.Total); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) ClaimDueEvents(ctx context.Context, limit int) ([]models.Event, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT e.id
		FROM events e
		WHERE e.status IN ('pending', 'retrying')
		  AND e.next_attempt_at <= NOW()
		ORDER BY e.next_attempt_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE events
		SET status = 'processing', updated_at = NOW()
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}

	out := make([]models.Event, 0, len(ids))
	for _, id := range ids {
		row := tx.QueryRow(ctx, `
			SELECT e.id, e.tenant_id, e.endpoint_id, e.event_type, e.payload, e.idempotency_key, e.status,
			       e.attempt_count, e.max_attempts, e.next_attempt_at, e.last_error, e.created_at, e.updated_at,
			       ep.url, ep.secret, ep.name
			FROM events e
			JOIN endpoints ep ON ep.id = e.endpoint_id
			WHERE e.id = $1
		`, id)
		var e models.Event
		if err := row.Scan(
			&e.ID, &e.TenantID, &e.EndpointID, &e.EventType, &e.Payload, &e.IdempotencyKey, &e.Status,
			&e.AttemptCount, &e.MaxAttempts, &e.NextAttemptAt, &e.LastError, &e.CreatedAt, &e.UpdatedAt,
			&e.EndpointURL, &e.EndpointSecret, &e.EndpointName,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetEventForDelivery(ctx context.Context, eventID uuid.UUID) (*models.Event, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT e.id, e.tenant_id, e.endpoint_id, e.event_type, e.payload, e.idempotency_key, e.status,
		       e.attempt_count, e.max_attempts, e.next_attempt_at, e.last_error, e.created_at, e.updated_at,
		       ep.url, ep.secret, ep.name
		FROM events e
		JOIN endpoints ep ON ep.id = e.endpoint_id
		WHERE e.id = $1
	`, eventID)

	var e models.Event
	if err := row.Scan(
		&e.ID, &e.TenantID, &e.EndpointID, &e.EventType, &e.Payload, &e.IdempotencyKey, &e.Status,
		&e.AttemptCount, &e.MaxAttempts, &e.NextAttemptAt, &e.LastError, &e.CreatedAt, &e.UpdatedAt,
		&e.EndpointURL, &e.EndpointSecret, &e.EndpointName,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (s *Store) MarkProcessing(ctx context.Context, eventID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE events
		SET status = 'processing', updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'retrying')
	`, eventID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

type AttemptResult struct {
	Success    bool
	HTTPStatus int
	Body       string
	LatencyMS  int
	Error      string
}

func backoffDuration(attempt int) time.Duration {
	// 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s (capped)
	secs := 1 << (attempt - 1)
	if secs > 128 {
		secs = 128
	}
	return time.Duration(secs) * time.Second
}

func (s *Store) RecordAttempt(ctx context.Context, event *models.Event, result AttemptResult) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	attemptNumber := event.AttemptCount + 1
	attemptStatus := models.StatusFailed
	if result.Success {
		attemptStatus = models.StatusDelivered
	}

	var httpStatus *int
	if result.HTTPStatus > 0 {
		httpStatus = &result.HTTPStatus
	}
	var body *string
	if result.Body != "" {
		b := truncate(result.Body, 2000)
		body = &b
	}
	var errText *string
	if result.Error != "" {
		e := truncate(result.Error, 1000)
		errText = &e
	}
	latency := result.LatencyMS

	_, err = tx.Exec(ctx, `
		INSERT INTO delivery_attempts (event_id, attempt_number, status, http_status, response_body, latency_ms, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, event.ID, attemptNumber, attemptStatus, httpStatus, body, latency, errText)
	if err != nil {
		return err
	}

	if result.Success {
		_, err = tx.Exec(ctx, `
			UPDATE events
			SET status = 'delivered',
			    attempt_count = $2,
			    last_error = NULL,
			    updated_at = NOW()
			WHERE id = $1
		`, event.ID, attemptNumber)
	} else if attemptNumber >= event.MaxAttempts {
		_, err = tx.Exec(ctx, `
			UPDATE events
			SET status = 'dead_letter',
			    attempt_count = $2,
			    last_error = $3,
			    updated_at = NOW()
			WHERE id = $1
		`, event.ID, attemptNumber, errText)
	} else {
		next := time.Now().Add(backoffDuration(attemptNumber))
		_, err = tx.Exec(ctx, `
			UPDATE events
			SET status = 'retrying',
			    attempt_count = $2,
			    next_attempt_at = $3,
			    last_error = $4,
			    updated_at = NOW()
			WHERE id = $1
		`, event.ID, attemptNumber, next, errText)
	}
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) ReplayEvent(ctx context.Context, tenantID, eventID uuid.UUID) (*models.Event, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx, `
		UPDATE events
		SET status = 'pending',
		    next_attempt_at = NOW(),
		    last_error = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND status IN ('dead_letter', 'failed', 'retrying', 'delivered')
	`, eventID, tenantID)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	job, _ := json.Marshal(models.DeliveryJob{EventID: eventID})
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (event_id, payload) VALUES ($1, $2)
	`, eventID, job)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetEvent(ctx, tenantID, eventID)
}

// PromoteDueRetries moves due retrying events back to pending and returns their IDs for Kafka publish.
func (s *Store) PromoteDueRetries(ctx context.Context, limit int) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE events
		SET status = 'pending', updated_at = NOW()
		WHERE id IN (
			SELECT id
			FROM events
			WHERE status = 'retrying'
			  AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ClaimOutbox(ctx context.Context, limit int) ([]uuid.UUID, []json.RawMessage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, payload
		FROM outbox
		WHERE published = FALSE
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, err
	}

	var ids []uuid.UUID
	var payloads []json.RawMessage
	for rows.Next() {
		var id uuid.UUID
		var payload json.RawMessage
		if err := rows.Scan(&id, &payload); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
		payloads = append(payloads, payload)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		_ = tx.Commit(ctx)
		return nil, nil, nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE outbox
		SET published = TRUE, published_at = NOW()
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return ids, payloads, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
