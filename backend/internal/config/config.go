package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL       string
	QueueDriver       string
	KafkaBrokers      []string
	KafkaTopic        string
	APIAddr           string
	WorkerConcurrency int
	MockReceiverAddr  string
	CORSOrigins       []string
	HTTPTimeout       time.Duration
}

func Load() Config {
	return Config{
		DatabaseURL:       getenv("DATABASE_URL", "postgres://webhook:webhook@localhost:5432/webhook?sslmode=disable"),
		QueueDriver:       strings.ToLower(getenv("QUEUE_DRIVER", "postgres")),
		KafkaBrokers:      splitCSV(getenv("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:        getenv("KAFKA_TOPIC", "webhook.deliveries"),
		APIAddr:           getenv("API_ADDR", ":8080"),
		WorkerConcurrency: getenvInt("WORKER_CONCURRENCY", 8),
		MockReceiverAddr:  getenv("MOCK_RECEIVER_ADDR", ":8090"),
		CORSOrigins: splitCSV(getenv("CORS_ORIGINS",
			"http://localhost:5173,http://localhost:4173,https://ranjitha-rani.github.io")),
		HTTPTimeout:       time.Duration(getenvInt("HTTP_TIMEOUT_MS", 5000)) * time.Millisecond,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
