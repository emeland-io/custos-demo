package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notebook/internal/auth"
)

func testFlowCodec() *auth.FlowCodec {
	return auth.NewFlowCodec(
		[]byte("fedcba9876543210fedcba9876543210"),
		[]byte("fedcba9876543210fedcba9876543210"),
	)
}

func TestFlowCodec_RoundTrip(t *testing.T) {
	codec := testFlowCodec()
	rec := httptest.NewRecorder()

	if err := codec.SetFlow(rec, "state-123", "verifier-456"); err != nil {
		t.Fatalf("set flow failed: %v", err)
	}

	req := &http.Request{Header: http.Header{"Cookie": rec.Header()["Set-Cookie"]}}
	state, verifier, ok := codec.ReadFlow(req)
	if !ok || state != "state-123" || verifier != "verifier-456" {
		t.Fatalf("expected state/verifier round trip, got state=%q verifier=%q ok=%v", state, verifier, ok)
	}
}

func TestFlowCodec_MissingCookie_IsRejected(t *testing.T) {
	codec := testFlowCodec()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, _, ok := codec.ReadFlow(req); ok {
		t.Fatalf("expected missing flow cookie to be rejected")
	}
}

func TestNewPKCEVerifier_And_Challenge_AreConsistent(t *testing.T) {
	verifier, err := auth.NewPKCEVerifier()
	if err != nil {
		t.Fatalf("failed to generate verifier: %v", err)
	}
	if len(verifier) < 43 {
		t.Fatalf("expected verifier of at least 43 chars per RFC 7636, got %d", len(verifier))
	}

	challenge1 := auth.PKCEChallenge(verifier)
	challenge2 := auth.PKCEChallenge(verifier)
	if challenge1 != challenge2 {
		t.Fatalf("expected deterministic challenge for the same verifier")
	}
	if challenge1 == verifier {
		t.Fatalf("expected challenge to differ from verifier")
	}
}

func TestNewState_ProducesUniqueValues(t *testing.T) {
	s1, err := auth.NewState()
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}
	s2, err := auth.NewState()
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}
	if s1 == s2 {
		t.Fatalf("expected two calls to NewState to produce different values")
	}
}
