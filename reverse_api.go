package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ReverseAPIClient is a lightweight HTTP client for the
// github.com/SOME-1HING/google-reverse-image-api service.
//
// Configuration (via environment variables):
// - REVERSE_API_URL:  Full POST endpoint (e.g., https://google-reverse-image-api.vercel.app/reverse)
// - REVERSE_API_BASE: Base URL; if REVERSE_API_URL not set, uses BASE + "/reverse"
// - REVERSE_API_KEY:  Optional API key (sent as Authorization: Bearer <key>)
// - REVERSE_API_TIMEOUT: Optional request timeout in seconds (default: 30)
//
// The client performs a POST with JSON body: {"imageUrl": "<image URL>"}
// and returns raw JSON data (map[string]any) for maximum flexibility.
type ReverseAPIClient struct {
	Endpoint string
	APIKey   string
	Client   *http.Client
}

// NewReverseAPIClient builds a client from environment variables.
func NewReverseAPIClient() (*ReverseAPIClient, error) {
	endpoint := strings.TrimSpace(os.Getenv("REVERSE_API_URL"))
	if endpoint == "" {
		base := strings.TrimRight(os.Getenv("REVERSE_API_BASE"), "/")
		if base == "" {
			return nil, fmt.Errorf("set REVERSE_API_URL or REVERSE_API_BASE (for /reverse endpoint)")
		}
		endpoint = base + "/reverse"
	}

	// Timeout
	to := 30 * time.Second
	if s := strings.TrimSpace(os.Getenv("REVERSE_API_TIMEOUT")); s != "" {
		if d, err := time.ParseDuration(s + "s"); err == nil {
			to = d
		}
	}

	return &ReverseAPIClient{
		Endpoint: endpoint,
		APIKey:   os.Getenv("REVERSE_API_KEY"),
		Client:   &http.Client{Timeout: to},
	}, nil
}

// ReverseSearch submits an image URL to the reverse image API and returns
// the raw JSON response. This helper uses a client created from environment vars.
func ReverseSearch(imageURL string) (map[string]any, error) {
	cli, err := NewReverseAPIClient()
	if err != nil {
		return nil, err
	}
	return cli.ReverseSearch(imageURL)
}

// ReverseSearch performs the reverse image lookup using POST only.
// Matches the example in example.py:
//
//	POST {endpoint}
//	Body: {"imageUrl": "<image URL>"}
func (c *ReverseAPIClient) ReverseSearch(imageURL string) (map[string]any, error) {
	if strings.TrimSpace(imageURL) == "" {
		return nil, fmt.Errorf("imageURL is empty")
	}
	payload := map[string]any{"imageUrl": imageURL}
	data, status, err := c.postJSON(c.Endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("reverse search failed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from reverse API", status)
	}
	return data, nil
}

// postJSON performs a POST with JSON payload and decodes JSON response into a generic map.
func (c *ReverseAPIClient) postJSON(u string, payload map[string]any) (map[string]any, int, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("encode json: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if k := strings.TrimSpace(c.APIKey); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	var out map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			if resp.StatusCode == http.StatusOK {
				return nil, resp.StatusCode, fmt.Errorf("decode json: %w", err)
			}
			return nil, resp.StatusCode, fmt.Errorf("non-OK status %d and invalid JSON: %v", resp.StatusCode, err)
		}
	}
	return out, resp.StatusCode, nil
}
