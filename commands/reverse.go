package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// RegisterReverse registers the /reverse command definition
func RegisterReverse(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "reverse",
		Description: "Performs a reverse image search on an Image URL",
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
		log.Printf("cannot create command reverse (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: reverse (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleReverse handles the /reverse command interaction
func HandleReverse(s *discordgo.Session, i *discordgo.InteractionCreate, reverseSearcher ReverseSearcher, isOwner func(string) bool, invokingUserID func(*discordgo.InteractionCreate) string, footerText, footerIconURL string) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "reverse" {
		return
	}
	// Make this command owner-only
	if !isOwner(invokingUserID(i)) {
		_ = RespondEphemeral(s, i, "Only the bot owner can use this command.")
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
		log.Println("failed to defer reverse interaction:", err)
		return
	}
	res, err := reverseSearcher.ReverseLookup(imageURL)
	if err != nil {
		msg := fmt.Sprintf("Reverse image search failed: %v", err)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}
	color := 0x607D8B
	desc := fmt.Sprintf("Reverse image search for: %s", imageURL)
	fields := []*discordgo.MessageEmbedField{
		{Name: "Success", Value: fmt.Sprintf("%t", res.Success), Inline: true},
	}
	if res.ResultText != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Result", Value: res.ResultText, Inline: false})
	}
	if res.SimilarURL != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Similar Results", Value: res.SimilarURL, Inline: false})
	}
	embed := &discordgo.MessageEmbed{Title: "Reverse Image Search", Description: desc, Color: color, Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}
