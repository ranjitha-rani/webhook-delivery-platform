# Webhook Delivery Platform

Reliable multi-tenant webhook delivery service

**Links:** [GitHub](https://github.com/ranjitha-rani/webhook-delivery-platform) · [Live UI](https://ranjitha-rani.github.io/webhook-delivery-platform/) — ingest events, sign and deliver HTTP callbacks, retry with backoff, dead-letter failures, and inspect everything from a React console.

**Stack:** Go · PostgreSQL · Kafka (optional) · React · TypeScript · Docker

---

## Architecture

```text
Producers / React Console
           |
           v
      Go Ingest API
           |
           +-- transactional write --> Postgres (events, attempts, outbox)
           |
           +-- QUEUE_DRIVER=postgres --> SKIP LOCKED job claimer
           +-- QUEUE_DRIVER=kafka    --> outbox relay --> Kafka --> workers
           |
           v
     Delivery Workers
      (HMAC-SHA256 signed POST)
           |
           +--> 2xx            --> delivered
           +--> retryable err  --> exponential backoff
           +--> max attempts   --> dead_letter (+ manual replay)
```

---

## Live demo (local)

```bash
# one-time
brew install docker docker-compose colima
colima start --cpu 2 --memory 3 --disk 20

cd webhook-delivery-platform
docker-compose up -d          # Postgres
make build

# 4 terminals
FAIL_RATE=0.3 DELAY_MS=50 ./bin/mock-receiver   # flaky customer endpoint
make api
make worker
make frontend-dev
```

Open **http://localhost:5173**  
API key: `dev-tenant-key`  
Endpoint URL: `http://localhost:8090/webhook`

1. Register endpoint → Send `order.paid`  
2. Watch status move to `delivered` (or `retrying` then delivered)  
3. Open event → retry timeline → **Replay / requeue**

### Kafka mode (optional)

```bash
docker-compose --profile kafka up -d
QUEUE_DRIVER=kafka make api
QUEUE_DRIVER=kafka make worker
```

### Benchmark

```bash
FAIL_RATE=0 DELAY_MS=0 ./bin/mock-receiver
make api && make worker
BENCH_N=500 BENCH_C=50 make bench
```

---

## GitHub Pages (frontend)

Live UI: https://ranjitha-rani.github.io/webhook-delivery-platform/

Pages is served from the `gh-pages` branch (static Vite build). The hosted UI shows **demo data** by default (browsers block HTTPS Pages → `http://localhost` API calls).

For a **live** console, use local Vite instead of Pages:

```bash
make api && make worker && make mock && make frontend-dev
# open http://localhost:5173
```

---

## API (auth: `X-API-Key`)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | health + queue driver |
| GET/POST | `/api/v1/endpoints` | list/create endpoints |
| GET | `/api/v1/endpoints/health` | per-endpoint success/latency |
| PATCH | `/api/v1/endpoints/{id}` | enable/disable |
| GET/POST | `/api/v1/events` | list/create events |
| GET | `/api/v1/events/{id}` | event + attempt timeline |
| POST | `/api/v1/events/{id}/replay` | requeue |
| GET | `/api/v1/stats` | status counts |

---

## Project layout

```text
backend/     Go API, worker, mock-receiver, bench
frontend/    React + TypeScript console (Vite)
scripts/     smoke test
NOTES.md     architecture, measured numbers, demo script
```

## Measured results (local Colima)

| Scenario | Result |
|----------|--------|
| Ingest (500 events, c=50, reliable mock) | **677.9 events/sec** |
| End-to-end delivery | **277.4 events/sec** |
| Eventual delivery w/ 30% injected failures | **100%** |
