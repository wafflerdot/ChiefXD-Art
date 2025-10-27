package main

// Default thresholds
const (
	DefaultNuditySuggestiveThreshold = 0.75
	DefaultNudityExplicitThreshold   = 0.25
	DefaultOffensiveThreshold        = 0.25
	DefaultAIGeneratedThreshold      = 0.60
)

// Analysis is a summary of the API result
// - Allowed: general verdict (true = no flags, false = flagged)
// - Reasons: list of flagged reasons
// - Scores: normalised scores
// - MediaURI: optional URI of the analysed media
type Analysis struct {
	Allowed bool
	Reasons []string

	Scores struct {
		// Explicit nudity score (sexual_activity, sexual_display, erotica)
		NudityExplicit float64
		// Suggestive nudity score (very_suggestive, suggestive, mildly_suggestive)
		NuditySuggestive float64
		Offensive        float64
		AIGenerated      float64
	}
	MediaURI string
}

// AdvancedAnalysis captures all numeric sub‑scores by category
type AdvancedAnalysis struct {
	Categories map[string]map[string]float64 // e.g. "nudity" -> {"none":0.95, "suggestive":0.02, ...}
	MediaURI   string
}

// AnalyseImageURL runs the API request via sightengine and analyses the result
func AnalyseImageURL(guildID, imageURL string) (*Analysis, error) {
	out, err := sightengine(imageURL)
	if err != nil {
		return nil, err
	}
	// Normalise raw response into an Analysis struct using guild-specific thresholds
	ns, ne, off, ai := thresholdsStore.GetGuildThresholds(perms, guildID)
	a := AnalyseResult(out, ns, ne, off, ai)
	return a, nil
}

// AnalyseImageURLAdvanced runs the API request via sightengine and returns full category/subcategory scores
func AnalyseImageURLAdvanced(imageURL string) (*AdvancedAnalysis, error) {
	out, err := sightengine(imageURL)
	if err != nil {
		return nil, err
	}
	return AnalyseResultAdvanced(out), nil
}

// AnalyseImageURLAIOnly runs the AI-only API request via sightengine and analyses the result
func AnalyseImageURLAIOnly(guildID, imageURL string) (*Analysis, error) {
	out, err := sightengineAIOnly(imageURL)
	if err != nil {
		return nil, err
	}
	ns, ne, off, ai := thresholdsStore.GetGuildThresholds(perms, guildID)
	return AnalyseResult(out, ns, ne, off, ai), nil
}

// AnalyseTempFile loads a local JSON result (e.g., 'temp.json') and analyses it
// DEV TESTING ONLY
//func AnalyseTempFile(path string) (*Analysis, error) {
//	b, err := os.ReadFile(path)
//	if err != nil {
//		return nil, fmt.Errorf("read %s: %w", path, err)
//	}
//	var out map[string]any
//	if err := json.Unmarshal(b, &out); err != nil {
//		return nil, fmt.Errorf("decode json: %w", err)
//	}
//	a := AnalyseResult(out)
//	return a, nil
//}

// AnalyseResult converts the raw map into an Analysis summary using provided thresholds
func AnalyseResult(out map[string]any, nsThresh, neThresh, offThresh, aiThresh float64) *Analysis {
	a := &Analysis{}

	// Extract scores
	// Nudity scores separated into explicit and suggestive
	nudity := getMap(out, "nudity")

	// Explicit
	a.Scores.NudityExplicit = maxFloat(
		getFloat(nudity, "sexual_activity"),
		getFloat(nudity, "sexual_display"),
		getFloat(nudity, "erotica"),
	)

	// Suggestive
	a.Scores.NuditySuggestive = meanFloat(
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
	)

	// AI-generated content score
	typ := getMap(out, "type")
	a.Scores.AIGenerated = getFloat(typ, "ai_generated")

	// Media URI
	if media := getMap(out, "media"); media != nil {
		if uri, ok := media["uri"].(string); ok {
			a.MediaURI = uri
		}
	}

	// Build reasons from thresholds
	if a.Scores.NudityExplicit >= neThresh {
		a.Reasons = append(a.Reasons, "nudity_explicit")
	}
	if a.Scores.NuditySuggestive >= nsThresh {
		a.Reasons = append(a.Reasons, "nudity_suggestive")
	}
	if a.Scores.Offensive >= offThresh {
		a.Reasons = append(a.Reasons, "offensive_symbols")
	}
	if a.Scores.AIGenerated >= aiThresh {
		a.Reasons = append(a.Reasons, "ai_generated_high")
	}

	// Safe when no rule produced a reason
	a.Allowed = len(a.Reasons) == 0
	return a
}

// AnalyseResultAdvanced extracts every numeric sub‑score from known categories and counts text arrays
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

// getMap returns m[key] as a map if present, otherwise returns nil
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

// getFloat extracts a numeric value from m[key] across common JSON-decoded types
// Returns 0 if the key is missing or not a number
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

// extractNumericSubscores returns all numeric leaf values from a category map
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

// Returns mean (average) of inputted values
func meanFloat(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
