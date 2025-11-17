package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// RegisterAI registers the /ai command definition
func RegisterAI(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ai",
		Description: "Checks an Image URL for AI usage",
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
			Description: "The Image URL to check",
			Required:    true,
		}},
	})
	if err != nil {
		log.Printf("cannot create command ai (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: ai (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleAI handles the /ai command interaction
func HandleAI(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, analyser ImageAnalyser, footerText, footerIconURL string) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "ai" {
		return
	}
	if !perms.IsAllowedForRestricted(i) {
		_ = RespondEphemeral(s, i, "You don't have permission to use this command.")
		return
	}

	var imageURL string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "image_url" {
			imageURL = opt.StringValue()
		}
	}
	if imageURL == "" {
		_ = RespondEphemeral(s, i, "Missing `image_url`.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer ai interaction:", err)
		return
	}
	analysis, err := analyser.AnalyseAIOnly(i.GuildID, imageURL)
	if err != nil {
		msg := fmt.Sprintf("AI check failed: %v", err)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "Safe Image", Value: fmt.Sprintf("%t", analysis.Allowed), Inline: true},
		{Name: "AI Generated", Value: fmt.Sprintf("%.0f%%", analysis.Scores.AIGenerated*100), Inline: true},
	}
	embed := &discordgo.MessageEmbed{Title: "AI Usage Check", Description: fmt.Sprintf("Analysis results for: %s", imageURL), Color: 0x3F51B5,
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}
