package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

type Server struct {
	store       *store.Store
	corsOrigins []string
	queueDriver string
}

func NewServer(st *store.Store, corsOrigins []string, queueDriver string) *Server {
	return &Server{store: st, corsOrigins: corsOrigins, queueDriver: queueDriver}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"queue_driver": s.queueDriver,
	})
}

type createEndpointReq struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func (s *Server) createEndpoint(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	var req createEndpointReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	if req.Secret == "" {
		req.Secret = randomSecret(24)
	}
	ep, err := s.store.CreateEndpoint(r.Context(), tenant.ID, req.Name, req.URL, req.Secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ep)
}

func (s *Server) listEndpoints(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	eps, err := s.store.ListEndpoints(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if eps == nil {
		eps = []models.Endpoint{}
	}
	writeJSON(w, http.StatusOK, eps)
}

func (s *Server) endpointHealth(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	health, err := s.store.ListEndpointHealth(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if health == nil {
		health = []models.EndpointHealth{}
	}
	writeJSON(w, http.StatusOK, health)
}

type patchEndpointReq struct {
	IsActive *bool `json:"is_active"`
}

func (s *Server) patchEndpoint(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req patchEndpointReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IsActive == nil {
		writeError(w, http.StatusBadRequest, "is_active boolean required")
		return
	}
	ep, err := s.store.SetEndpointActive(r.Context(), tenant.ID, id, *req.IsActive)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "endpoint not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ep)
}

type createEventReq struct {
	EndpointID     string          `json:"endpoint_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	var req createEventReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	endpointID, err := uuid.Parse(req.EndpointID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint_id")
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	if req.EventType == "" {
		writeError(w, http.StatusBadRequest, "event_type is required")
		return
	}
	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage(`{}`)
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = randomSecret(16)
	}

	ev, existed, err := s.store.CreateEventWithOutbox(r.Context(), store.CreateEventInput{
		TenantID:       tenant.ID,
		EndpointID:     endpointID,
		EventType:      req.EventType,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "endpoint not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusAccepted
	if existed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"event":   ev,
		"replayed": existed,
	})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	status := r.URL.Query().Get("status")
	events, err := s.store.ListEvents(r.Context(), tenant.ID, status, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []models.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ev, err := s.store.GetEvent(r.Context(), tenant.ID, id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	attempts, err := s.store.ListAttempts(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":    ev,
		"attempts": attempts,
	})
}

func (s *Server) replayEvent(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ev, err := s.store.ReplayEvent(r.Context(), tenant.ID, id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "event not found or not replayable")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, ev)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	st, err := s.store.Stats(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func randomSecret(n int) string {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return hex.EncodeToString([]byte("fallback-secret"))
	}
	return hex.EncodeToString(b)
}
