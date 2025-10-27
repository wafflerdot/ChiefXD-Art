package main

import (
	"errors"
	"fmt"
)

// ReverseResult is a compact, convenient representation of the
// google-reverse-image-api response, suitable for further processing
//
// This intentionally flattens out the nested JSON so callers don't need
// to navigate maps or nested structs. Unknown fields from the upstream
// API are ignored on purpose to keep this resilient to schema changes
type ReverseResult struct {
	// Success is the upstream API's success boolean.
	Success bool
	// Message is the upstream API's message string.
	Message string
	// SimilarURL is a Google Images search URL for visually similar results.
	SimilarURL string
	// ResultText is a short description string returned by the API.
	ResultText string
}

// AsReverseResultRaw converts a generic decoded JSON object (map[string]any)
// returned by ReverseSearch into a ReverseResult. This avoids re-encoding
// the map form is already available
func AsReverseResultRaw(raw map[string]any) (*ReverseResult, error) {
	if raw == nil {
		return nil, errors.New("nil raw map")
	}
	getMap := func(m map[string]any, k string) map[string]any {
		if v, ok := m[k]; ok {
			if mm, ok := v.(map[string]any); ok {
				return mm
			}
		}
		return nil
	}
	getString := func(m map[string]any, k string) string {
		if m == nil {
			return ""
		}
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	res := &ReverseResult{}
	if v, ok := raw["success"].(bool); ok {
		res.Success = v
	}
	res.Message = getString(raw, "message")
	data := getMap(raw, "data")
	res.SimilarURL = getString(data, "similarUrl")
	res.ResultText = getString(data, "resultText")
	return res, nil
}

// ReverseLookup is a convenience that runs the network call via ReverseSearch
// and returns the normalised ReverseResult ready for higher-level use
func ReverseLookup(imageURL string) (*ReverseResult, error) {
	raw, err := ReverseSearch(imageURL)
	if err != nil {
		return nil, err
	}
	return AsReverseResultRaw(raw)
}

// String returns a compact human-readable rendering (useful for logs)
func (r *ReverseResult) String() string {
	if r == nil {
		return "<nil>"
	}
	return fmt.Sprintf("success=%t message=%q similarUrl=%q resultText=%q", r.Success, r.Message, r.SimilarURL, r.ResultText)
}
