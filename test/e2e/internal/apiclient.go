package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
)

// dexHostPort is the hostname:port Dex's issuer URL embeds in the
// authorization redirect it sends to the client driving the OIDC
// flow. That hostname only resolves inside the cluster's own DNS, so
// a client running outside the cluster (this Go client, or a real
// browser) must be told where to actually reach it instead.
const dexHostPort = "dex.notebook-e2e.svc.cluster.local:5556"

// NewCookieClient returns an http.Client with a cookie jar (so the
// OIDC session cookie set by /auth/callback survives across
// requests), whose Transport routes Dex's cluster-internal issuer
// hostname to dexLocalAddr — typically a kubectl port-forward to
// Dex's Service — since that hostname is otherwise unreachable from
// outside the cluster.
func NewCookieClient(dexLocalAddr string) *http.Client {
	jar, _ := cookiejar.New(nil)
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == dexHostPort {
				addr = dexLocalAddr
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{Jar: jar, Transport: transport}
}

var formActionPattern = regexp.MustCompile(`<form[^>]+action="([^"]+)"`)

// LoginViaDex drives the full browserless login round trip: GET
// /auth/login (which redirects through our backend's OIDC flow to
// Dex's static login form), scrapes that form's action URL, submits
// the given credentials, and follows Dex's redirect back through
// /auth/callback — leaving an authenticated session cookie in
// client's cookie jar.
func LoginViaDex(client *http.Client, baseURL, email, password string) error {
	resp, err := client.Get(baseURL + "/auth/login")
	if err != nil {
		return fmt.Errorf("fetching login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading login page: %w", err)
	}

	matches := formActionPattern.FindSubmatch(body)
	if matches == nil {
		return fmt.Errorf("could not find login form action in Dex's login page")
	}
	actionURL, err := resp.Request.URL.Parse(html.UnescapeString(string(matches[1])))
	if err != nil {
		return fmt.Errorf("resolving form action URL: %w", err)
	}

	form := url.Values{"login": {email}, "password": {password}}
	submitResp, err := client.PostForm(actionURL.String(), form)
	if err != nil {
		return fmt.Errorf("submitting Dex login form: %w", err)
	}
	defer submitResp.Body.Close()

	if submitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(submitResp.Body)
		return fmt.Errorf("unexpected status %d after Dex login submission: %s", submitResp.StatusCode, b)
	}
	return nil
}

type NoteSummary struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

func CreateNote(client *http.Client, baseURL, title, content string) (*NoteSummary, error) {
	body, _ := json.Marshal(map[string]string{"title": title, "content": content})
	resp, err := client.Post(baseURL+"/api/notes", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d creating note: %s", resp.StatusCode, b)
	}
	var note NoteSummary
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		return nil, err
	}
	return &note, nil
}

func ListNotes(client *http.Client, baseURL string) ([]NoteSummary, error) {
	resp, err := client.Get(baseURL + "/api/notes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Items []NoteSummary `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Items, nil
}
