# Project Notes

## What this is
A multi-tenant **webhook delivery platform** (Go + Postgres + optional Kafka + React) that ingests events, delivers signed HTTP callbacks, retries with backoff, dead-letters exhausted deliveries, and exposes a console for inspection/replay.

## Architecture (implemented)
```text
React console / producers
        |
        v
Go Ingest API  --(tx)-->  events + outbox (Postgres)
        |
        +--> QUEUE_DRIVER=postgres --> SKIP LOCKED claimer
        +--> QUEUE_DRIVER=kafka    --> outbox relay --> Kafka --> workers
        |
        v
Delivery workers (HMAC sign + POST)
        |
        +--> success -> delivered
        +--> retry   -> exponential backoff
        +--> give up -> dead_letter (+ manual replay)
```

## Measured numbers (local, Colima, Apple Silicon)

### Throughput (reliable mock: `FAIL_RATE=0`, `DELAY_MS=0`)
| Metric | Value |
|--------|------:|
| Events | 500 |
| Publish concurrency | 50 |
| Ingest rate | **677.9 events/sec** |
| End-to-end delivery rate | **277.4 events/sec** |
| Eventual delivery | **100%** |
| Dead letters | 0 |
| Elapsed | 1.80s |
| Measured at | 2026-08-10T21:18:27Z |

### Reliability (flaky mock: `FAIL_RATE=0.3`, `DELAY_MS=50`)
| Metric | Value |
|--------|------:|
| Events | 300 |
| Publish concurrency | 30 |
| Eventual delivery | **100%** |
| Dead letters | 0 |
| Avg attempt latency | ~50.8 ms |
| Elapsed to full delivery | 9.08s |
| Measured at | 2026-08-10T21:19:06Z |

> Re-run anytime with `make bench` after starting api/worker/mock. Results land in `bench-results.json`.

## Demo script (2 minutes)
1. `colima start && docker-compose up -d && make build`
2. Terminals: `FAIL_RATE=0.3 make mock` · `make api` · `make worker` · `make frontend-dev`
3. UI `http://localhost:5173` → register `http://localhost:8090/webhook` → send `order.paid`
4. Show retry timeline / delivered status / Replay button
5. Optional: `BENCH_N=500 BENCH_C=50 make bench`

## GitHub Pages
- Workflow: `.github/workflows/deploy-pages.yml`
- Hosted UI talks to local API (`http://localhost:8080`) via Connection panel
- Enable: repo Settings → Pages → GitHub Actions

## Verification checklist
- [x] Postgres queue path (UI demo + bench)
- [x] Kafka queue path (`docker-compose --profile kafka` + smoke → delivered)
- [x] HMAC signing + retries + DLQ + replay
- [x] React console + endpoint health
- [x] CI workflow + Pages workflow
- [x] Push to GitHub: https://github.com/ranjitha-rani/webhook-delivery-platform
- [x] GitHub Pages UI: https://ranjitha-rani.github.io/webhook-delivery-platform/

## What’s intentionally out of MVP
- Kubernetes / cloud Kafka
- Exactly-once side effects at the customer
- Per-tenant rate-limit enforcement beyond API key isolation
- Paid managed hosting
