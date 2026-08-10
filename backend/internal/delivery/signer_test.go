package delivery

import (
	"testing"
	"time"
)

func TestSignDeterministic(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	timestamp, sig := Sign("secret", []byte(`{"ok":true}`), ts)
	if timestamp != "1700000000" {
		t.Fatalf("timestamp=%s", timestamp)
	}
	if sig == "" {
		t.Fatal("expected signature")
	}
	_, sig2 := Sign("secret", []byte(`{"ok":true}`), ts)
	if sig != sig2 {
		t.Fatal("signatures should match")
	}
	_, sig3 := Sign("other", []byte(`{"ok":true}`), ts)
	if sig == sig3 {
		t.Fatal("different secrets should differ")
	}
}
