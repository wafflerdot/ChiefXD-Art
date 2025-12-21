package commands

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// RegisterPing registers the /ping command definition
func RegisterPing(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Pong!",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
			discordgo.ApplicationIntegrationUserInstall,
		},
	})
	if err != nil {
		log.Printf("cannot create command ping (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: ping (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandlePing handles the /ping command interaction
func HandlePing(s *discordgo.Session, i *discordgo.InteractionCreate, footerText, footerIconURL string) {
	start := time.Now()
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer ping:", err)
		return
	}
	rtt := time.Since(start)
	gw := s.HeartbeatLatency()
	embed := &discordgo.MessageEmbed{Title: "Pong!", Color: 0xFFC107,
		Fields: []*discordgo.MessageEmbedField{{
			Name:   "Response time",
			Value:  fmt.Sprintf("%d ms", rtt.Milliseconds()),
			Inline: true,
		},
			{Name: "API latency",
				Value:  fmt.Sprintf("%d ms", gw.Milliseconds()),
				Inline: true,
			},
		}, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}
