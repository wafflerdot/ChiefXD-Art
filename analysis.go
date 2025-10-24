package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Tunable thresholds for policy checks.
// 1.00 = 100% certain flag; 0.00 = no flag.
const (
	NuditySuggestiveThreshold = 0.50
	NudityExplicitThreshold   = 0.50
	OffensiveThreshold        = 0.50
	AIGeneratedThreshold      = 0.50
)

// Analysis is a compact, comparable summary of the API result.
// - Allowed: general verdict (true = no flags, false = flagged)
// - Reasons: list of flagged reasons
// - Scores: normalised scores
// - TextCounts: counts of flags in each category
// - MediaURI: optional URI of the analysed media
type Analysis struct {
	Allowed bool
	Reasons []string

	Scores struct {
		Nudity      float64
		Offensive   float64
		AIGenerated float64
	}
	MediaURI string
}

// AdvancedAnalysis captures all numeric sub‑scores by category, plus text counts.
type AdvancedAnalysis struct {
	Categories map[string]map[string]float64 // e.g., "nudity" -> {"none":0.95, "suggestive":0.02, ...}
	MediaURI   string
}

// AnalyseImageURL runs the API request via sightengine and analyses the result.
func AnalyseImageURL(imageURL string) (*Analysis, error) {
	out, err := sightengine(imageURL)
	if err != nil {
		return nil, err
	}
	a := AnalyseResult(out)
	return a, nil
}

// AnalyseImageURLAdvanced runs the API request via sightengine and returns full category/subcategory scores.
func AnalyseImageURLAdvanced(imageURL string) (*AdvancedAnalysis, error) {
	out, err := sightengine(imageURL)
	if err != nil {
		return nil, err
	}
	return AnalyseResultAdvanced(out), nil
}

// AnalyseTempFile loads a local JSON result (e.g., 'temp.json') and analyses it.
// DEV TESTING ONLY
func AnalyseTempFile(path string) (*Analysis, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	a := AnalyseResult(out)
	return a, nil
}

// AnalyseResult converts the raw map into an Analysis summary using fixed thresholds.
func AnalyseResult(out map[string]any) *Analysis {
	a := &Analysis{}

	// Extract scores
	// Nudity score
	nudity := getMap(out, "nudity")
	// Prefer 1 - none as a single nudity score; fallback to max of other classes.
	a.Scores.Nudity = maxFloat(1.0-getFloat(nudity, "none"),
		getFloat(nudity, "sexual_activity"),
		getFloat(nudity, "sexual_display"),
		getFloat(nudity, "erotica"),
		getFloat(nudity, "very_suggestive"),
		getFloat(nudity, "suggestive"),
		getFloat(nudity, "mildly_suggestive"),
	)

	// Offensive symbols score
	off := getMap(out, "offensive")
	a.Scores.Offensive = maxFloat(
		getFloat(off, "nazi"),
		getFloat(off, "asian_swastika"),
		getFloat(off, "confederate"),
		getFloat(off, "supremacist"),
		getFloat(off, "terrorist"),
		getFloat(off, "middle_finger"),
	)

	// AI-generated content score
	typ := getMap(out, "type")
	a.Scores.AIGenerated = getFloat(typ, "ai_generated")

	// Media URI (optional)
	if media := getMap(out, "media"); media != nil {
		if uri, ok := media["uri"].(string); ok {
			a.MediaURI = uri
		}
	}

	// Build reasons from thresholds
	if a.Scores.Nudity >= NudityExplicitThreshold {
		a.Reasons = append(a.Reasons, "nudity_explicit")
	} else if a.Scores.Nudity >= NuditySuggestiveThreshold {
		a.Reasons = append(a.Reasons, "nudity_suggestive")
	}
	if a.Scores.Offensive >= OffensiveThreshold {
		a.Reasons = append(a.Reasons, "offensive_symbols")
	}
	if a.Scores.AIGenerated >= AIGeneratedThreshold {
		a.Reasons = append(a.Reasons, "ai_generated_high")
	}

	// Allowed when no rule produced a reason.
	a.Allowed = len(a.Reasons) == 0
	return a
}

// AnalyseResultAdvanced extracts every numeric sub‑score from known categories and counts text arrays.
func AnalyseResultAdvanced(out map[string]any) *AdvancedAnalysis {
	aa := &AdvancedAnalysis{
		Categories: make(map[string]map[string]float64),
	}

	for _, k := range []string{"nudity", "offensive", "type"} {
		if mm := getMap(out, k); mm != nil {
			if subs := extractNumericSubscores(mm); len(subs) > 0 {
				aa.Categories[k] = subs
			}
		}
	}

	// Media URI (optional)
	if media := getMap(out, "media"); media != nil {
		if uri, ok := media["uri"].(string); ok {
			aa.MediaURI = uri
		}
	}

	return aa
}

// Helpers

// getMap returns m\[key] as a map if present, otherwise nil.
// Safe to call with nil m; never panics.
func getMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}

// getFloat extracts a numeric value from m\[key] across common JSON-decoded types.
// Returns 0 if the key is missing or not a number.
func getFloat(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case float64:
			return t
		case float32:
			return float64(t)
		case int:
			return float64(t)
		case int64:
			return float64(t)
		}
	}
	return 0
}

// extractNumericSubscores returns all numeric leaf values from a category map.
func extractNumericSubscores(m map[string]any) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range m {
		switch t := v.(type) {
		case float64:
			out[k] = t
		case float32:
			out[k] = float64(t)
		case int:
			out[k] = float64(t)
		case int64:
			out[k] = float64(t)
		}
	}
	return out
}

// Returns maximum of inputted values
func maxFloat(vals ...float64) float64 {
	maximum := 0.0
	for _, v := range vals {
		if v > maximum {
			maximum = v
		}
	}
	return maximum
}
