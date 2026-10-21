package main

import (
	"ChiefXD-Art/api"
	"ChiefXD-Art/commands"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

const (
	monitoredGuildID   = "800047499904679936"
	monitoredChannelID = "818805988311302166"
	// 1246210655715856546, 1541977578234187859
	alertsChannelID = "1541962886455365632"
)

var whitelistedRoleIDs = map[string]struct{}{
	"823119803778727956": {},
}

var imageURLPattern = regexp.MustCompile(`https?://[^\s)>'"]+`)

// startupComplete is set to true after bot initialisation is complete
// This prevents onGuildCreate from running during initial guild sync
var startupComplete atomic.Bool

// StartupComplete signals that bot initialisation is done
// and onGuildCreate should start processing new guild joins
func StartupComplete() {
	startupComplete.Store(true)
	log.Println("Bot startup complete - now listening for new guild joins")
}

// registerHandlers wires all slash command handlers onto the session
func registerHandlers(sess *discordgo.Session) {
	// Apply Rich Presence on READY
	sess.AddHandler(onReadySetPresence)

	// Monitor the configured guild/channel for media uploads and moderation alerts
	sess.AddHandler(onMessageCreateMonitor)

	// Register commands when bot joins a new guild
	sess.AddHandler(onGuildCreate)

	// Command handler dispatcher
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.GuildID == "" && !IsOwner(invokingUserID(i)) {
			_ = commands.RespondEphemeral(s, i, "This command is only available to bot owners in DMs")
			return
		}
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if dbLessMode {
				switch i.ApplicationCommandData().Name {
				case "permissions", "thresholds", "whitelist":
					_ = commands.RespondEphemeral(s, i, "This command is disabled when running in DB-less mode.")
					return
				}
			}
			// Route to the appropriate command handler based on command name
			switch i.ApplicationCommandData().Name {
			case "ping":
				commands.HandlePing(s, i, FooterText, FooterIconURL)
			case "help":
				commands.HandleHelp(s, i, FooterText, FooterIconURL, dbLessMode)
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
			if dbLessMode {
				_ = commands.RespondEphemeral(s, i, "This command is disabled when running in DB-less mode.")
				return
			}
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

func onMessageCreateMonitor(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil || m.Author.Bot {
		return
	}
	if m.GuildID != monitoredGuildID || m.ChannelID != monitoredChannelID {
		return
	}
	if isWhitelistedMember(m.Member) {
		return
	}

	imageURLs := extractImageURLs(m)
	if len(imageURLs) == 0 {
		return
	}

	for _, imageURL := range imageURLs {
		result, err := AnalyseImageURL(m.GuildID, imageURL)
		if err != nil {
			log.Printf("monitor: full scan failed for %s: %v", imageURL, err)
			continue
		}
		if result == nil || result.Allowed {
			continue
		}
		go sendMonitoringAlert(s, m, imageURL, result)
	}
}

func isWhitelistedMember(member *discordgo.Member) bool {
	if member == nil {
		return false
	}
	for _, roleID := range member.Roles {
		if _, ok := whitelistedRoleIDs[roleID]; ok {
			return true
		}
	}
	return false
}

func extractImageURLs(m *discordgo.MessageCreate) []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0, 4)
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		if !isImageURL(u) {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	for _, attachment := range m.Attachments {
		if attachment == nil {
			continue
		}
		if attachment.ContentType != "" && strings.HasPrefix(strings.ToLower(attachment.ContentType), "image/") {
			add(attachment.URL)
			continue
		}
		add(attachment.URL)
	}
	for _, embed := range m.Embeds {
		if embed == nil {
			continue
		}
		if embed.Image != nil {
			add(embed.Image.URL)
		}
		if embed.Thumbnail != nil {
			add(embed.Thumbnail.URL)
		}
	}
	for _, match := range imageURLPattern.FindAllString(m.Content, -1) {
		add(match)
	}
	return urls
}

func isImageURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, ".png") || strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") || strings.Contains(lower, ".gif") || strings.Contains(lower, ".webp") || strings.Contains(lower, ".avif") || strings.Contains(lower, ".bmp") {
		return true
	}
	if strings.Contains(lower, "image/") || strings.Contains(lower, "cdn.discordapp.com") || strings.Contains(lower, "media.discordapp.net") {
		return true
	}
	return false
}

func sendMonitoringAlert(s *discordgo.Session, m *discordgo.MessageCreate, imageURL string, result *api.Analysis) {
	if alertsChannelID == "" || alertsChannelID == "SET_ALERTS_CHANNEL_ID_HERE" {
		log.Printf("monitor: alerts channel not configured; skipping AI alert for message %s", m.ID)
		return
	}
	if s == nil || s.Token == "" {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Image Moderation Alert",
		Description: fmt.Sprintf("Automated moderation flagged media posted in <#%s>.", monitoredChannelID),
		Color:       0xFF4D4D,
		Image:       &discordgo.MessageEmbedImage{URL: imageURL},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Author", Value: fmt.Sprintf("<@%s>", m.Author.ID), Inline: true},
			{Name: "AI Generated", Value: fmt.Sprintf("%.0f%%", result.Scores.AIGenerated*100), Inline: true},
			{Name: "Nudity (Explicit)", Value: fmt.Sprintf("%.0f%%", result.Scores.NudityExplicit*100), Inline: true},
			{Name: "Nudity (Suggestive)", Value: fmt.Sprintf("%.0f%%", result.Scores.NuditySuggestive*100), Inline: true},
			{Name: "Offensive", Value: fmt.Sprintf("%.0f%%", result.Scores.Offensive*100), Inline: true},
			{Name: "Triggered Checks", Value: formatTriggeredReasons(result.Reasons), Inline: false},
			{Name: "Jump", Value: fmt.Sprintf("[Open message](https://discord.com/channels/%s/%s/%s)", monitoredGuildID, monitoredChannelID, m.ID), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL},
	}

	if _, err := s.ChannelMessageSendComplex(alertsChannelID, &discordgo.MessageSend{Embed: embed}); err != nil {
		log.Printf("monitor: failed to send moderation alert for message %s: %v", m.ID, err)
	}
}

func formatTriggeredReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "none"
	}
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		switch reason {
		case "nudity_explicit":
			out = append(out, "Nudity (Explicit)")
		case "nudity_suggestive":
			out = append(out, "Nudity (Suggestive)")
		case "offensive_symbols":
			out = append(out, "Offensive")
		case "ai_generated_high":
			out = append(out, "AI Generated")
		default:
			out = append(out, reason)
		}
	}
	return strings.Join(out, ", ")
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
