package commands

import "github.com/bwmarrin/discordgo"

// PermissionChecker provides permission checking capabilities
type PermissionChecker interface {
	IsAllowedForRestricted(i *discordgo.InteractionCreate) bool
	AddRole(guildID, roleID string)
	RemoveRole(guildID, roleID string)
	ListRoles(guildID string) []string
	AddWhitelistedUser(userID string)
	RemoveWhitelistedUser(userID string)
	ListWhitelistedUsers() []string
}

// ImageAnalyser provides image analysis capabilities
type ImageAnalyser interface {
	Analyse(guildID, imageURL string) (*AnalysisResult, error)
	AnalyseAdvanced(imageURL string) (*AdvancedAnalysisResult, error)
	AnalyseAIOnly(guildID, imageURL string) (*AnalysisResult, error)
}

// AnalysisResult represents standard analysis output
type AnalysisResult struct {
	Allowed bool
	Scores  AnalysisScores
}

// AnalysisScores contains the various detection scores
type AnalysisScores struct {
	NudityExplicit   float64
	NuditySuggestive float64
	Offensive        float64
	AIGenerated      float64
}

// AdvancedAnalysisResult represents advanced analysis output with categories
type AdvancedAnalysisResult struct {
	Categories map[string]map[string]float64
}

// ThresholdStore provides threshold management
type ThresholdStore interface {
	GetGuildThresholds(ps interface{}, guildID string) (nuditySuggestive, nudityExplicit, offensive, aiGenerated float64)
	SetGuild(ps interface{}, guildID, name string, value float64) error
	ResetOneGuild(ps interface{}, guildID, name string) error
	ResetAllGuild(ps interface{}, guildID string) error
	LogChange(ps interface{}, name string, oldVal, newVal float64, userID, guildID string) error
	HistoryForGuild(ps interface{}, guildID string, limit int) ([]ThresholdChange, error)
	HistoryFilteredForGuild(ps interface{}, guildID, name string, limit int) ([]ThresholdChange, error)
}

// ThresholdChange represents a historical threshold modification
type ThresholdChange struct {
	Name     string
	OldValue NullFloat64
	NewValue float64
	UserID   NullString
	Created  interface{} // time.Time
}

// NullFloat64 represents a nullable float64
type NullFloat64 struct {
	Float64 float64
	Valid   bool
}

// NullString represents a nullable string
type NullString struct {
	String string
	Valid  bool
}

// ReverseSearcher provides reverse image search capabilities
type ReverseSearcher interface {
	ReverseLookup(imageURL string) (*ReverseSearchResult, error)
}

// ReverseSearchResult represents the result from a reverse image search
type ReverseSearchResult struct {
	Success    bool
	ResultText string
	SimilarURL string
}

// RespondEphemeral sends an ephemeral message visible only to the invoking user
func RespondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
