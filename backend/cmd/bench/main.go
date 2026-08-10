package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Load / reliability bench against a running API + worker + mock receiver.
//
// Example:
//
//	go run ./cmd/bench -n 500 -c 50
func main() {
	apiBase := getenv("API_BASE", "http://localhost:8080")
	apiKey := getenv("API_KEY", "dev-tenant-key")
	endpointURL := getenv("ENDPOINT_URL", "http://localhost:8090/webhook")
	n := getenvInt("BENCH_N", 500)
	concurrency := getenvInt("BENCH_C", 50)

	client := &http.Client{Timeout: 10 * time.Second}

	fmt.Printf("bench config: n=%d concurrency=%d api=%s\n", n, concurrency, apiBase)

	epBody := map[string]any{
		"name":   fmt.Sprintf("bench-%d", time.Now().Unix()),
		"url":    endpointURL,
		"secret": "bench-secret",
	}
	epResp, err := doJSON(client, http.MethodPost, apiBase+"/api/v1/endpoints", apiKey, epBody)
	must(err)
	endpointID, _ := epResp["id"].(string)
	if endpointID == "" {
		fatalf("create endpoint failed: %v", epResp)
	}
	fmt.Println("endpoint:", endpointID)

	var accepted atomic.Int64
	var publishErrs atomic.Int64
	start := time.Now()

	jobs := make(chan int, n)
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				body := map[string]any{
					"endpoint_id":     endpointID,
					"event_type":      "bench.event",
					"idempotency_key": fmt.Sprintf("bench-%d-%d", start.UnixNano(), idx),
					"payload":         map[string]any{"i": idx},
				}
				_, err := doJSON(client, http.MethodPost, apiBase+"/api/v1/events", apiKey, body)
				if err != nil {
					publishErrs.Add(1)
					continue
				}
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	ingestElapsed := time.Since(start)
	ingestRate := float64(accepted.Load()) / ingestElapsed.Seconds()
	fmt.Printf("ingest: accepted=%d errors=%d elapsed=%s rate=%.1f events/sec\n",
		accepted.Load(), publishErrs.Load(), ingestElapsed.Truncate(time.Millisecond), ingestRate)

	fmt.Println("waiting for terminal delivery states (endpoint-scoped)...")
	deadline := time.Now().Add(3 * time.Minute)
	var delivered, dead, retrying int
	var avgLatency float64
	for time.Now().Before(deadline) {
		healthList, err := doJSONList(client, apiBase+"/api/v1/endpoints/health", apiKey)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, h := range healthList {
			id, _ := h["id"].(string)
			if id != endpointID {
				continue
			}
			delivered = asInt(h["delivered"])
			dead = asInt(h["dead_letter"])
			retrying = asInt(h["retrying"])
			avgLatency, _ = h["avg_latency_ms"].(float64)
		}
		totalTerminal := delivered + dead
		fmt.Printf("\r  delivered=%d dead_letter=%d retrying=%d terminal=%d/%d",
			delivered, dead, retrying, totalTerminal, accepted.Load())
		if totalTerminal >= int(accepted.Load()) && retrying == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()
	totalElapsed := time.Since(start)
	target := float64(accepted.Load())
	eventual := 0.0
	if target > 0 {
		eventual = 100.0 * float64(delivered) / target
	}
	deliveryRate := float64(delivered) / totalElapsed.Seconds()

	fmt.Println("---- results ----")
	fmt.Printf("accepted_events:       %d\n", accepted.Load())
	fmt.Printf("delivered:             %d\n", delivered)
	fmt.Printf("dead_letter:           %d\n", dead)
	fmt.Printf("eventual_delivery%%:    %.2f\n", eventual)
	fmt.Printf("ingest_events_per_s:   %.1f\n", ingestRate)
	fmt.Printf("delivery_events_per_s: %.1f\n", deliveryRate)
	fmt.Printf("avg_latency_ms:        %.1f\n", avgLatency)
	fmt.Printf("total_elapsed:         %s\n", totalElapsed.Truncate(time.Millisecond))

	// Write machine-readable summary for NOTES / resume updates.
	summary := map[string]any{
		"accepted":                accepted.Load(),
		"delivered":               delivered,
		"dead_letter":             dead,
		"eventual_delivery_pct":   eventual,
		"ingest_events_per_sec":   ingestRate,
		"delivery_events_per_sec": deliveryRate,
		"avg_latency_ms":          avgLatency,
		"concurrency":             concurrency,
		"elapsed_ms":              totalElapsed.Milliseconds(),
		"measured_at":             time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile("bench-results.json", b, 0o644)
	fmt.Println("wrote bench-results.json")
}

func doJSONList(client *http.Client, url, apiKey string) ([]map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(raw))
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func doJSON(client *http.Client, method, url, apiKey string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(raw))
	}
	var out map[string]any
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		// list endpoints returns array sometimes; ignore
		return map[string]any{}, nil
	}
	return out, nil
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		return 0
	}
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

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
