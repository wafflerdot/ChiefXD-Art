package commands

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// RegisterThresholds registers the /thresholds command definition
func RegisterThresholds(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "thresholds",
		Description: "Shows or modifies detection thresholds",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
		},
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
		},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List current detection thresholds",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set",
				Description: "Set a threshold (0.00-1.00 or a percentage like 70%)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "threshold",
						Description: "Select which threshold to set",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "Suggestive Nudity", Value: "NuditySuggestive"},
							{Name: "Explicit Nudity", Value: "NudityExplicit"},
							{Name: "Offensive Content", Value: "Offensive"},
							{Name: "AI Generated", Value: "AIGenerated"},
						},
					},
					{Type: discordgo.ApplicationCommandOptionString, Name: "value", Description: "Decimal (0.00-1.00) or percentage (0-100%)", Required: true},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset",
				Description: "Reset thresholds to default",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "threshold",
						Description: "Select threshold to reset (or 'All to reset all')",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "Suggestive Nudity", Value: "NuditySuggestive"},
							{Name: "Explicit Nudity", Value: "NudityExplicit"},
							{Name: "Offensive Content", Value: "Offensive"},
							{Name: "AI Generated", Value: "AIGenerated"},
							{Name: "All (DANGER)", Value: "all"},
						},
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "history",
				Description: "Show recent threshold changes",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "limit", Description: "How many recent changes to show (1-100)", Required: false},
					{Type: discordgo.ApplicationCommandOptionString, Name: "threshold", Description: "Filter by threshold name", Required: false,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "NuditySuggestive", Value: "NuditySuggestive"},
							{Name: "NudityExplicit", Value: "NudityExplicit"},
							{Name: "Offensive", Value: "Offensive"},
							{Name: "AIGenerated", Value: "AIGenerated"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("cannot create command thresholds (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: thresholds (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleThresholds handles the /thresholds command interaction
func HandleThresholds(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, thresholds ThresholdStore, isOwner func(string) bool, hasAdminPerm func(*discordgo.InteractionCreate) bool, invokingUserID func(*discordgo.InteractionCreate) string, defaultThresholdValue func(string) float64, footerText, footerIconURL string) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "thresholds" {
		return
	}

	data := i.ApplicationCommandData()
	guildID := i.GuildID

	// If no subcommand or list => view only (allowed roles can view)
	if len(data.Options) == 0 || data.Options[0].Name == "list" {
		if !(hasAdminPerm(i) || perms.IsAllowedForRestricted(i)) {
			_ = RespondEphemeral(s, i, "You don't have permission to view thresholds.")
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer thresholds:", err)
			return
		}
		ns, ne, off, ai := thresholds.GetGuildThresholds(perms, guildID)
		val := fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			ne*100, ns*100, off*100, ai*100)
		embed := &discordgo.MessageEmbed{Title: "Detection Thresholds", Description: "Current thresholds to flag image", Color: 0x9C27B0,
			Fields: []*discordgo.MessageEmbedField{{Name: "Thresholds", Value: val, Inline: false}}, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// History: view-only (allowed roles or admins)
	if data.Options[0].Name == "history" {
		if !(hasAdminPerm(i) || perms.IsAllowedForRestricted(i)) {
			_ = RespondEphemeral(s, i, "You don't have permission to view threshold history.")
			return
		}
		limit := 10
		var nameFilter string
		for _, opt := range data.Options[0].Options {
			if opt.Name == "limit" {
				limit = int(opt.IntValue())
			}
			if opt.Name == "threshold" {
				nameFilter = strings.TrimSpace(opt.StringValue())
			}
		}
		var (
			changes []ThresholdChange
			err     error
		)
		if nameFilter != "" {
			canonical, ok := CanonicalThresholdName(nameFilter)
			if !ok {
				_ = RespondEphemeral(s, i, "Unknown threshold filter. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
				return
			}
			changes, err = thresholds.HistoryFilteredForGuild(perms, guildID, canonical, limit)
		} else {
			changes, err = thresholds.HistoryForGuild(perms, guildID, limit)
		}
		if err != nil {
			log.Println("thresholds history error:", err)
			_ = RespondEphemeral(s, i, "Failed to fetch history")
			return
		}
		if len(changes) == 0 {
			_ = RespondEphemeral(s, i, "No history available.")
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer thresholds history:", err)
			return
		}
		fields := make([]*discordgo.MessageEmbedField, 0, len(changes))
		for _, c := range changes {
			user := "unknown"
			if c.UserID.Valid {
				user = "<@" + c.UserID.String + ">"
			}
			old := "n/a"
			if c.OldValue.Valid {
				old = fmt.Sprintf("%.2f%%", c.OldValue.Float64*100)
			}
			created := c.Created.(time.Time)
			val := fmt.Sprintf("%s\nOld: %s → New: %.2f%%\nBy: %s\nAt: %s",
				c.Name, old, c.NewValue*100, user, created.Format(time.RFC3339))
			fields = append(fields, &discordgo.MessageEmbedField{Name: "Change", Value: val, Inline: false})
		}
		embed := &discordgo.MessageEmbed{Title: "Thresholds History", Color: 0x8E44AD, Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// set/reset require owner/admin privileges
	if !(isOwner(invokingUserID(i)) || hasAdminPerm(i)) {
		_ = RespondEphemeral(s, i, "Only server admins or the owner can modify thresholds.")
		return
	}

	sub := data.Options[0]
	switch sub.Name {
	case "set":
		var name, valueStr string

		// Parse value: allow percent like 70% or 0.70
		for _, opt := range sub.Options {
			if opt.Name == "threshold" {
				name = strings.TrimSpace(opt.StringValue())
			}
			if opt.Name == "value" {
				// Normalise name variants
				valueStr = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" || valueStr == "" {
			_ = RespondEphemeral(s, i, "Usage: /thresholds set <Name> <Value>")
			return
		}
		val, err := ParseThresholdValue(valueStr)
		if err != nil || val < 0 || val > 1 {
			_ = RespondEphemeral(s, i, "Value must be a decimal between 0.00 and 1.00, or a percentage like 70%")
			return
		}
		canonical, ok := CanonicalThresholdName(name)
		if !ok {
			_ = RespondEphemeral(s, i, "Unknown threshold. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
			return
		}
		oldNS, oldNE, oldOff, oldAI := thresholds.GetGuildThresholds(perms, guildID)
		oldMap := map[string]float64{"NuditySuggestive": oldNS, "NudityExplicit": oldNE, "Offensive": oldOff, "AIGenerated": oldAI}
		if err := thresholds.SetGuild(perms, guildID, canonical, val); err != nil {
			log.Println("thresholds set guild error:", err)
			_ = RespondEphemeral(s, i, "Failed to update threshold")
			return
		}
		_ = thresholds.LogChange(perms, canonical, oldMap[canonical], val, invokingUserID(i), guildID)
		msg := fmt.Sprintf("Set %s to %.2f%%", canonical, val*100)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})

	case "reset":
		var name string
		for _, opt := range sub.Options {
			if opt.Name == "threshold" {
				name = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" {
			_ = RespondEphemeral(s, i, "Usage: /thresholds reset <Name|all>")
			return
		}
		if strings.EqualFold(name, "all") {
			oldNS, oldNE, oldOff, oldAI := thresholds.GetGuildThresholds(perms, guildID)
			if err := thresholds.ResetAllGuild(perms, guildID); err != nil {
				log.Println("thresholds reset all guild error:", err)
				_ = RespondEphemeral(s, i, "Failed to reset thresholds")
				return
			}
			_ = thresholds.LogChange(perms, "NuditySuggestive", oldNS, defaultThresholdValue("NuditySuggestive"), invokingUserID(i), guildID)
			_ = thresholds.LogChange(perms, "NudityExplicit", oldNE, defaultThresholdValue("NudityExplicit"), invokingUserID(i), guildID)
			_ = thresholds.LogChange(perms, "Offensive", oldOff, defaultThresholdValue("Offensive"), invokingUserID(i), guildID)
			_ = thresholds.LogChange(perms, "AIGenerated", oldAI, defaultThresholdValue("AIGenerated"), invokingUserID(i), guildID)
			msg := "Reset all thresholds to default"
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: msg}})
			return
		}
		canonical, ok := CanonicalThresholdName(name)
		if !ok {
			_ = RespondEphemeral(s, i, "Unknown threshold. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
			return
		}
		oldNS, oldNE, oldOff, oldAI := thresholds.GetGuildThresholds(perms, guildID)
		oldMap := map[string]float64{"NuditySuggestive": oldNS, "NudityExplicit": oldNE, "Offensive": oldOff, "AIGenerated": oldAI}
		if err := thresholds.ResetOneGuild(perms, guildID, canonical); err != nil {
			log.Println("thresholds reset one guild error:", err)
			_ = RespondEphemeral(s, i, "Failed to reset threshold")
			return
		}
		// after reset, new value equals built-in default
		_ = thresholds.LogChange(perms, canonical, oldMap[canonical], defaultThresholdValue(canonical), invokingUserID(i), guildID)
		msg := fmt.Sprintf("Reset %s to default", canonical)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})
	}
}

// ParseThresholdValue parses a threshold value from a string (decimal or percentage)
func ParseThresholdValue(in string) (float64, error) {
	s := strings.TrimSpace(in)
	if strings.HasSuffix(s, "%") {
		p := strings.TrimSuffix(s, "%")
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, err
		}
		return f / 100.0, nil
	}
	return strconv.ParseFloat(s, 64)
}

// CanonicalThresholdName normalizes threshold name variants to canonical form
func CanonicalThresholdName(in string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(in))
	switch s {
	case "nuditysuggestive", "suggestive", "nudity_suggestive":
		return "NuditySuggestive", true
	case "nudityexplicit", "explicit", "nudity_explicit":
		return "NudityExplicit", true
	case "offensive", "offensive_symbols", "offensivesymbols":
		return "Offensive", true
	case "aigenerated", "ai", "genai", "ai_generated":
		return "AIGenerated", true
	default:
		return "", false
	}
}
