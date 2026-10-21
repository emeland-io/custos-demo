package internal_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"notebook/test/e2e/internal"
)

// Dex's OIDC authorization endpoint is built from its issuer hostname
// (dex.<namespace>.svc.cluster.local), which only resolves inside the
// cluster's own DNS. The app's /auth/login redirect sends that URL to
// whatever client is actually driving the flow — here, a plain Go
// http.Client running on the host, outside the cluster — so the
// client needs a way to reach that hostname without the cluster's
// DNS. NewCookieClient takes a host:port to route that one hostname
// to (a local port-forward to Dex's Service), proven here against a
// real local listener rather than a mock.
func TestNewCookieClient_RoutesDexHostnameToLocalAddr(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open a test listener: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	client := internal.NewCookieClient(ln.Addr().String())

	resp, err := client.Get("http://dex.notebook-e2e.svc.cluster.local:5556/anything")
	if err != nil {
		t.Fatalf("expected the request to be routed to the local listener, got error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("expected status %d from the local listener, got %d", http.StatusTeapot, resp.StatusCode)
	}
}

// Dex's server-rendered login form HTML-escapes its action attribute
// (e.g. action="/auth/local/login?back=&amp;state=abc123"), since
// that's correct HTML. LoginViaDex scrapes that attribute with a
// regex and resolves it as a URL; if it doesn't HTML-unescape the
// scraped string first, the literal "&amp;" is parsed as part of the
// query string, turning "state=abc123" into "amp;state=abc123" and
// losing Dex's own state/req correlation — which is exactly the "Bad
// Request: User session error" Dex itself returns in that case.
func TestLoginViaDex_UnescapesHTMLEntitiesInFormAction(t *testing.T) {
	var gotState string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotState = r.URL.Query().Get("state")
			w.WriteHeader(http.StatusOK)
			return
		}
		fmt.Fprint(w, `<html><body><form method="post" action="/auth/local/login?back=&amp;state=abc123">`+
			`<input name="login"><input name="password"></form></body></html>`)
	}))
	defer fake.Close()

	client := fake.Client()
	if err := internal.LoginViaDex(client, fake.URL, "user@example.com", "password"); err != nil {
		t.Fatalf("LoginViaDex failed: %v", err)
	}
	if gotState != "abc123" {
		t.Fatalf("expected the POST to carry state=abc123 (decoded), got query state=%q", gotState)
	}
}
