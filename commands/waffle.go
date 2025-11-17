package commands

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// RegisterWaffle registers the /waffle command definition
func RegisterWaffle(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "waffle",
		Description: "Waffle",
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
			Name:        "message",
			Description: "Message to print",
			Required:    true,
		}},
	})
	if err != nil {
		log.Printf("cannot create command waffle (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: waffle (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleWaffle handles the /waffle command interaction
func HandleWaffle(s *discordgo.Session, i *discordgo.InteractionCreate, isOwner func(string) bool, invokingUserID func(*discordgo.InteractionCreate) string) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "waffle" {
		return
	}
	if !isOwner(invokingUserID(i)) {
		_ = RespondEphemeral(s, i, "Only the bot owner can use this command")
		return
	}
	// Extract the message option
	var msg string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "message" {
			msg = opt.StringValue()
		}
	}
	if msg == "" {
		_ = RespondEphemeral(s, i, "Please provide a message")
		return
	}

	// Acknowledge ephemerally, then send the plain message to the channel
	if err := RespondEphemeral(s, i, "Message sent."); err != nil {
		// Fallback: if ephemeral isn't allowed (e.g., in DMs), acknowledge normally
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Message sent."},
		})
	}
	// Now send the message content unmodified as a normal message
	if _, err := s.ChannelMessageSend(i.ChannelID, msg); err != nil {
		log.Println("failed to send waffle message:", err)
	}
}
