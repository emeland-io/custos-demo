package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFlowCodec_StaleCookie_IsRejectedServerSide(t *testing.T) {
	original := flowTTL
	flowTTL = 1 * time.Second
	defer func() { flowTTL = original }()

	codec := NewFlowCodec(
		[]byte("fedcba9876543210fedcba9876543210"),
		[]byte("fedcba9876543210fedcba9876543210"),
	)
	rec := httptest.NewRecorder()
	if err := codec.SetFlow(rec, "state-123", "verifier-456"); err != nil {
		t.Fatalf("set flow failed: %v", err)
	}

	time.Sleep(2100 * time.Millisecond)

	req := &http.Request{Header: http.Header{"Cookie": rec.Header()["Set-Cookie"]}}
	if _, _, ok := codec.ReadFlow(req); ok {
		t.Fatalf("expected a flow cookie older than its TTL to be rejected server-side")
	}
}
