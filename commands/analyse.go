package commands

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// RegisterAnalyse registers the /analyse command definition
func RegisterAnalyse(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "analyse",
		Description: "Analyses an Image URL for inappropriate content",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
			discordgo.ApplicationIntegrationUserInstall,
		},
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "image_url",
			Description: "The Image URL to analyse",
			Required:    true,
		}, {
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        "advanced",
			Description: "Advanced mode, shows more detailed results",
			Required:    false,
		}},
	})
	if err != nil {
		log.Printf("cannot create command analyse (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: analyse (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleAnalyse handles the /analyse command interaction
func HandleAnalyse(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, analyser ImageAnalyser, footerText, footerIconURL string) {
	if !perms.IsAllowedForRestricted(i) {
		_ = RespondEphemeral(s, i, "You don't have permission to use this command.")
		return
	}

	// Extract options
	var (
		imageURL string
		advanced bool
	)
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "image_url":
			imageURL = opt.StringValue()
		case "advanced":
			advanced = opt.BoolValue()
		}
	}
	if imageURL == "" {
		_ = RespondEphemeral(s, i, "Missing `image_url`.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer interaction:", err)
		return
	}

	if advanced {
		aa, err := analyser.AnalyseAdvanced(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		formatScores := func(title string, m map[string]float64) *discordgo.MessageEmbedField {
			if len(m) == 0 {
				return &discordgo.MessageEmbedField{Name: title, Value: "none", Inline: false}
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
			var b strings.Builder
			for _, k := range keys {
				_, _ = fmt.Fprintf(&b, "%s: %.0f%%\n", k, m[k]*100)
			}
			val := strings.TrimRight(b.String(), "\n")
			return &discordgo.MessageEmbedField{Name: title, Value: val, Inline: false}
		}
		fields := make([]*discordgo.MessageEmbedField, 0, 6)
		if nudity, ok := aa.Categories["nudity"]; ok {
			fields = append(fields, formatScores("Nudity", nudity))
		}
		if offensive, ok := aa.Categories["offensive"]; ok {
			fields = append(fields, formatScores("Offensive Content", offensive))
		}
		if typ, ok := aa.Categories["type"]; ok {
			fields = append(fields, formatScores("AI Usage", typ))
		}
		embed := &discordgo.MessageEmbed{Title: "Image Analysis (Advanced)", Description: fmt.Sprintf("Analysis results for: %s", imageURL), Color: 0x4CAF50,
			Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// Standard
	a, err := analyser.Analyse(i.GuildID, imageURL)
	if err != nil {
		msg := fmt.Sprintf("Analysis failed: %v", err)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "Safe Image", Value: fmt.Sprintf("%t", a.Allowed), Inline: true},
		{Name: "Results", Value: fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			a.Scores.NudityExplicit*100, a.Scores.NuditySuggestive*100, a.Scores.Offensive*100, a.Scores.AIGenerated*100), Inline: false},
	}
	embed := &discordgo.MessageEmbed{Title: "Image Analysis", Description: fmt.Sprintf("Analysis results for: %s", imageURL), Color: 0x00BFA5,
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}
