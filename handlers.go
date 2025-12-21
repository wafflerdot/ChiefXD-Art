package main

import (
	"ChiefXD-Art/commands"
	"log"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

// startupComplete is set to true after bot initialization is complete
// This prevents onGuildCreate from running during initial guild sync
var startupComplete atomic.Bool

// StartupComplete signals that bot initialization is done
// and onGuildCreate should start processing new guild joins
func StartupComplete() {
	startupComplete.Store(true)
	log.Println("Bot startup complete - now listening for new guild joins")
}

// registerHandlers wires all slash command handlers onto the session
func registerHandlers(sess *discordgo.Session) {
	// Apply Rich Presence on READY
	sess.AddHandler(onReadySetPresence)

	// Register commands when bot joins a new guild
	sess.AddHandler(onGuildCreate)

	// Command handler dispatcher
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			// Route to the appropriate command handler based on command name
			switch i.ApplicationCommandData().Name {
			case "ping":
				commands.HandlePing(s, i, FooterText, FooterIconURL)
			case "help":
				commands.HandleHelp(s, i, FooterText, FooterIconURL)
			case "analyse":
				commands.HandleAnalyse(s, i, perms, &analysisAdapter{}, FooterText, FooterIconURL)
			case "ai":
				commands.HandleAI(s, i, perms, &analysisAdapter{}, FooterText, FooterIconURL)
			case "permissions":
				commands.HandlePermissions(s, i, perms, IsOwner, HasAdminContextPermission, invokingUserID, commands.FormatRoleList, FooterText, FooterIconURL)
			case "thresholds":
				commands.HandleThresholds(s, i, perms, &thresholdsAdapter{ps: perms}, IsOwner, HasAdminContextPermission, invokingUserID, defaultThresholdValue, FooterText, FooterIconURL)
			case "reverse":
				commands.HandleReverse(s, i, &reverseAdapter{}, IsOwner, invokingUserID, FooterText, FooterIconURL)
			case "whitelist":
				commands.HandleWhitelist(s, i, perms, IsOwner, invokingUserID, FooterText, FooterIconURL)
			case "waffle":
				commands.HandleWaffle(s, i, IsOwner, invokingUserID)
			}
		case discordgo.InteractionMessageComponent:
			// Handle component interactions (buttons, etc.)
			commands.HandleWhitelistComponents(s, i, perms, IsOwner, invokingUserID, FooterText, FooterIconURL)
		}
	})
}

// -------------------------
// Guild Join Handler
// -------------------------
// onGuildCreate is triggered when the bot joins a new guild or when the bot starts up
// and syncs all guilds. We only process this after startup is complete to avoid
// registering commands for existing guilds (which is handled in main).
func onGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	// Skip if bot is still initializing
	if !startupComplete.Load() {
		return
	}

	log.Printf("Bot joined new guild: %s (%s)", event.Guild.Name, event.Guild.ID)

	// Register all commands to this guild for instant availability
	if globalRegister {
		go func() {
			appID := s.State.User.ID
			createAllCommands(s, appID, event.Guild.ID)
			log.Printf("Commands registered for guild: %s", event.Guild.ID)
		}()
	}
}

// -------------------------
// Adapters to bridge main package to commands package
// -------------------------

// analysisAdapter implements commands.ImageAnalyser
type analysisAdapter struct{}

func (a *analysisAdapter) Analyse(guildID, imageURL string) (*commands.AnalysisResult, error) {
	result, err := AnalyseImageURL(guildID, imageURL)
	if err != nil {
		return nil, err
	}
	return &commands.AnalysisResult{
		Allowed: result.Allowed,
		Scores: commands.AnalysisScores{
			NudityExplicit:   result.Scores.NudityExplicit,
			NuditySuggestive: result.Scores.NuditySuggestive,
			Offensive:        result.Scores.Offensive,
			AIGenerated:      result.Scores.AIGenerated,
		},
	}, nil
}

func (a *analysisAdapter) AnalyseAdvanced(imageURL string) (*commands.AdvancedAnalysisResult, error) {
	result, err := AnalyseImageURLAdvanced(imageURL)
	if err != nil {
		return nil, err
	}
	return &commands.AdvancedAnalysisResult{
		Categories: result.Categories,
	}, nil
}

func (a *analysisAdapter) AnalyseAIOnly(guildID, imageURL string) (*commands.AnalysisResult, error) {
	result, err := AnalyseImageURLAIOnly(guildID, imageURL)
	if err != nil {
		return nil, err
	}
	return &commands.AnalysisResult{
		Allowed: result.Allowed,
		Scores: commands.AnalysisScores{
			NudityExplicit:   result.Scores.NudityExplicit,
			NuditySuggestive: result.Scores.NuditySuggestive,
			Offensive:        result.Scores.Offensive,
			AIGenerated:      result.Scores.AIGenerated,
		},
	}, nil
}

// reverseAdapter implements commands.ReverseSearcher
type reverseAdapter struct{}

func (r *reverseAdapter) ReverseLookup(imageURL string) (*commands.ReverseSearchResult, error) {
	result, err := ReverseLookup(imageURL)
	if err != nil {
		return nil, err
	}
	return &commands.ReverseSearchResult{
		Success:    result.Success,
		ResultText: result.ResultText,
		SimilarURL: result.SimilarURL,
	}, nil
}

// thresholdsAdapter implements commands.ThresholdStore
type thresholdsAdapter struct {
	ps *PermStore
}

func (t *thresholdsAdapter) GetGuildThresholds(ps interface{}, guildID string) (nuditySuggestive, nudityExplicit, offensive, aiGenerated float64) {
	return thresholdsStore.GetGuildThresholds(t.ps, guildID)
}

func (t *thresholdsAdapter) SetGuild(ps interface{}, guildID, name string, value float64) error {
	return thresholdsStore.SetGuild(t.ps, guildID, name, value)
}

func (t *thresholdsAdapter) ResetOneGuild(ps interface{}, guildID, name string) error {
	return thresholdsStore.ResetOneGuild(t.ps, guildID, name)
}

func (t *thresholdsAdapter) ResetAllGuild(ps interface{}, guildID string) error {
	return thresholdsStore.ResetAllGuild(t.ps, guildID)
}

func (t *thresholdsAdapter) LogChange(ps interface{}, name string, oldVal, newVal float64, userID, guildID string) error {
	return thresholdsStore.LogChange(t.ps, name, oldVal, newVal, userID, guildID)
}

func (t *thresholdsAdapter) HistoryForGuild(ps interface{}, guildID string, limit int) ([]commands.ThresholdChange, error) {
	changes, err := thresholdsStore.HistoryForGuild(t.ps, guildID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]commands.ThresholdChange, len(changes))
	for i, c := range changes {
		result[i] = commands.ThresholdChange{
			Name:     c.Name,
			OldValue: commands.NullFloat64{Float64: c.OldValue.Float64, Valid: c.OldValue.Valid},
			NewValue: c.NewValue,
			UserID:   commands.NullString{String: c.UserID.String, Valid: c.UserID.Valid},
			Created:  c.Created,
		}
	}
	return result, nil
}

func (t *thresholdsAdapter) HistoryFilteredForGuild(ps interface{}, guildID, name string, limit int) ([]commands.ThresholdChange, error) {
	changes, err := thresholdsStore.HistoryFilteredForGuild(t.ps, guildID, name, limit)
	if err != nil {
		return nil, err
	}
	result := make([]commands.ThresholdChange, len(changes))
	for i, c := range changes {
		result[i] = commands.ThresholdChange{
			Name:     c.Name,
			OldValue: commands.NullFloat64{Float64: c.OldValue.Float64, Valid: c.OldValue.Valid},
			NewValue: c.NewValue,
			UserID:   commands.NullString{String: c.UserID.String, Valid: c.UserID.Valid},
			Created:  c.Created,
		}
	}
	return result, nil
}
