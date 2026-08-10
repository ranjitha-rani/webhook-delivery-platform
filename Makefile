QUEUE_DRIVER ?= postgres

.PHONY: infra-up infra-down build api worker mock frontend-dev frontend-build test tidy smoke demo-help

infra-up:
	docker-compose up -d

infra-up-kafka:
	docker-compose --profile kafka up -d

infra-down:
	docker-compose down

tidy:
	cd backend && go mod tidy

build:
	mkdir -p bin
	cd backend && go build -o ../bin/api ./cmd/api
	cd backend && go build -o ../bin/worker ./cmd/worker
	cd backend && go build -o ../bin/mock-receiver ./cmd/mock-receiver
	cd backend && go build -o ../bin/bench ./cmd/bench

test:
	cd backend && go test ./...

# Requires api + worker + mock already running.
bench:
	cd backend && BENCH_N=$${BENCH_N:-500} BENCH_C=$${BENCH_C:-50} go run ./cmd/bench
	@mv -f backend/bench-results.json ./bench-results.json 2>/dev/null || true

bench-help:
	@echo "Reliable throughput:"
	@echo "  FAIL_RATE=0 DELAY_MS=0 ./bin/mock-receiver"
	@echo "  make api && make worker"
	@echo "  BENCH_N=500 BENCH_C=50 make bench"
	@echo "Failure / eventual delivery:"
	@echo "  FAIL_RATE=0.3 DELAY_MS=50 ./bin/mock-receiver"
	@echo "  BENCH_N=300 BENCH_C=30 make bench"

api:
	DATABASE_URL=postgres://webhook:webhook@localhost:5432/webhook?sslmode=disable \
	QUEUE_DRIVER=$(QUEUE_DRIVER) \
	KAFKA_BROKERS=localhost:9092 \
	CORS_ORIGINS=http://localhost:5173,http://localhost:4173 \
	./bin/api

worker:
	DATABASE_URL=postgres://webhook:webhook@localhost:5432/webhook?sslmode=disable \
	QUEUE_DRIVER=$(QUEUE_DRIVER) \
	KAFKA_BROKERS=localhost:9092 \
	./bin/worker

mock:
	FAIL_RATE=0.5 DELAY_MS=150 ./bin/mock-receiver

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

smoke:
	bash scripts/smoke.sh

demo-help:
	@echo "1) make infra-up"
	@echo "2) make build"
	@echo "3) make mock"
	@echo "4) make api"
	@echo "5) make worker"
	@echo "6) make frontend-dev"
	@echo "Optional Kafka mode: QUEUE_DRIVER=kafka make api / make worker"
	@echo "Dashboard: http://localhost:5173  API key: dev-tenant-key"
