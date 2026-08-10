package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Flaky webhook receiver for local demos.
// Query/env knobs:
//   FAIL_RATE=0.3     -> 30% chance of 500
//   DELAY_MS=200      -> artificial latency
//   REQUIRE_SECRET=.. -> validate X-Webhook-Signature when set

func main() {
	addr := getenv("MOCK_RECEIVER_ADDR", ":8090")
	failRate := getenvFloat("FAIL_RATE", 0.4)
	delayMS := getenvInt("DELAY_MS", 100)
	secret := os.Getenv("REQUIRE_SECRET")

	var received atomic.Int64
	var failed atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"status":   "ok",
			"received": received.Load(),
			"failed":   failed.Load(),
			"failRate": failRate,
		})
	})

	mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if delayMS > 0 {
			time.Sleep(time.Duration(delayMS) * time.Millisecond)
		}

		if secret != "" {
			ts := r.Header.Get("X-Webhook-Timestamp")
			sig := strings.TrimPrefix(r.Header.Get("X-Webhook-Signature"), "sha256=")
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(ts))
			mac.Write([]byte("."))
			mac.Write(body)
			expected := hex.EncodeToString(mac.Sum(nil))
			if !hmac.Equal([]byte(expected), []byte(sig)) {
				failed.Add(1)
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}

		// Deterministic flakiness from webhook id hash so retries eventually succeed.
		id := r.Header.Get("X-Webhook-Id")
		attemptHeader := r.Header.Get("X-Attempt")
		shouldFail := false
		if failRate > 0 {
			h := sha256.Sum256([]byte(id + attemptHeader + strconv.FormatInt(received.Load(), 10)))
			// First couple deliveries for an id tend to fail when failRate is high.
			roll := float64(h[0]) / 255.0
			shouldFail = roll < failRate && received.Load()%3 != 2
		}

		received.Add(1)
		if shouldFail {
			failed.Add(1)
			log.Printf("FAIL id=%s bytes=%d", id, len(body))
			http.Error(w, "injected failure", http.StatusBadGateway)
			return
		}

		log.Printf("OK   id=%s bytes=%d sig=%s", id, len(body), r.Header.Get("X-Webhook-Signature"))
		writeJSON(w, map[string]any{"ok": true, "id": id})
	})

	log.Printf("mock receiver on %s fail_rate=%.2f delay_ms=%d", addr, failRate, delayMS)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getenvInt(k string, d int) int {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func getenvFloat(k string, d float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return d
	}
	return n
}
