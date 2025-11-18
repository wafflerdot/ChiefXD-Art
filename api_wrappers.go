package main

import (
	"ChiefXD-Art/api"
)

// Wrapper functions to bridge main package to api package

// AnalyseImageURL runs the API request via api.Sightengine and analyses the result
func AnalyseImageURL(guildID, imageURL string) (*api.Analysis, error) {
	thresholdsGetter := func(gID string) (float64, float64, float64, float64) {
		return thresholdsStore.GetGuildThresholds(perms, gID)
	}
	return api.AnalyseImageURL(guildID, imageURL, thresholdsGetter)
}

// AnalyseImageURLAdvanced runs the API request via api.Sightengine and returns full category/subcategory scores
func AnalyseImageURLAdvanced(imageURL string) (*api.AdvancedAnalysis, error) {
	return api.AnalyseImageURLAdvanced(imageURL)
}

// AnalyseImageURLAIOnly runs the AI-only API request via api.Sightengine and analyses the result
func AnalyseImageURLAIOnly(guildID, imageURL string) (*api.Analysis, error) {
	thresholdsGetter := func(gID string) (float64, float64, float64, float64) {
		return thresholdsStore.GetGuildThresholds(perms, gID)
	}
	return api.AnalyseImageURLAIOnly(guildID, imageURL, thresholdsGetter)
}

// ReverseLookup is a convenience that runs the network call via api.ReverseSearch
// and returns the normalised api.ReverseResult ready for higher-level use
func ReverseLookup(imageURL string) (*api.ReverseResult, error) {
	return api.ReverseLookup(imageURL)
}
