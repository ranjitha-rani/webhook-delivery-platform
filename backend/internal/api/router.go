package api

import "net/http"

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/stats", s.withAuth(s.stats))
	mux.HandleFunc("GET /api/v1/endpoints", s.withAuth(s.listEndpoints))
	mux.HandleFunc("POST /api/v1/endpoints", s.withAuth(s.createEndpoint))
	mux.HandleFunc("GET /api/v1/endpoints/health", s.withAuth(s.endpointHealth))
	mux.HandleFunc("PATCH /api/v1/endpoints/{id}", s.withAuth(s.patchEndpoint))
	mux.HandleFunc("GET /api/v1/events", s.withAuth(s.listEvents))
	mux.HandleFunc("POST /api/v1/events", s.withAuth(s.createEvent))
	mux.HandleFunc("GET /api/v1/events/{id}", s.withAuth(s.getEvent))
	mux.HandleFunc("POST /api/v1/events/{id}/replay", s.withAuth(s.replayEvent))
	return s.withCORS(mux)
}
