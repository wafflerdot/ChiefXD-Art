package commands

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// RegisterHelp registers the /help command definition
func RegisterHelp(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows a list of commands",
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
		log.Printf("cannot create command help (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: help (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleHelp handles the /help command interaction
func HandleHelp(s *discordgo.Session, i *discordgo.InteractionCreate, footerText, footerIconURL string, dbLessMode bool) {
	fields := []*discordgo.MessageEmbedField{
		{Name: "/ai", Value: "Checks an Image URL for AI usage\nArguments: `image_url` (required)", Inline: false},
		{Name: "/analyse", Value: "Analyses an Image URL for inappropriate content\nArguments:\n- `image_url` (required)\n- `advanced` (optional): `true` shows detailed category and subcategory scores", Inline: false},
		{Name: "/help", Value: "Shows this message", Inline: false},
		{Name: "/ping", Value: "Displays the bot's response time", Inline: false},
		{Name: "/reverse", Value: "Performs a reverse image search on an Image URL\nArguments: `image_url` (required)", Inline: false},
	}
	if !dbLessMode {
		fields = append(fields,
			&discordgo.MessageEmbedField{Name: "/permissions", Value: "Manage which roles can use moderator-only commands (owner/admin only)", Inline: false},
			&discordgo.MessageEmbedField{Name: "/thresholds", Value: "Shows or modifies detection thresholds\nSubcommands:\n- `list`: View current thresholds\n- `history [limit] [threshold]`: View recent changes\n- `set <Threshold> <Value>`: Modify a detection threshold (owner/admin only)\n- `reset <Threshold|all>`: Resets a threshold to its default value (owner/admin only)", Inline: false},
			&discordgo.MessageEmbedField{Name: "/whitelist", Value: "Manage DM whitelist (owner only). Subcommands: `add <user>`, `remove <user>`, `list`", Inline: false},
		)
	}
	embed := &discordgo.MessageEmbed{Title: "Help", Description: "Available commands", Color: 0x5865F2,
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}

	// Use immediate response (single API call) instead of defer + edit (two API calls)
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}); err != nil {
		log.Println("failed to respond to help:", err)
	}
}
