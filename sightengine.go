package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// sightengine calls the Sightengine API with the full model set used by standard/advanced analysis
func sightengine(imageLink string) (map[string]any, error) {
	apiUser := os.Getenv("SIGHTENGINE_USER")
	apiSecret := os.Getenv("SIGHTENGINE_SECRET")
	if apiUser == "" || apiSecret == "" {
		return nil, fmt.Errorf("SIGHTENGINE_USER and SIGHTENGINE_SECRET must be set")
	}

	base := "https://api.sightengine.com/1.0/check.json"
	params := url.Values{}
	params.Set("url", imageLink)
	params.Set("models", "nudity-2.1,offensive-2.0,genai")
	params.Set("api_user", apiUser)
	params.Set("api_secret", apiSecret)

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.RawQuery = params.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return out, nil
}

// sightengineAIOnly calls the Sightengine API with the AI detection only model
func sightengineAIOnly(imageLink string) (map[string]any, error) {
	apiUser := os.Getenv("SIGHTENGINE_USER")
	apiSecret := os.Getenv("SIGHTENGINE_SECRET")
	if apiUser == "" || apiSecret == "" {
		return nil, fmt.Errorf("SIGHTENGINE_USER and SIGHTENGINE_SECRET must be set")
	}

	base := "https://api.sightengine.com/1.0/check.json"
	params := url.Values{}
	params.Set("url", imageLink)
	params.Set("models", "genai")
	params.Set("api_user", apiUser)
	params.Set("api_secret", apiSecret)

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.RawQuery = params.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return out, nil
}
