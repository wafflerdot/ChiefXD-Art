package main

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
)

// registerCommands creates slash commands globally and optionally to a dev guild
func registerCommands(sess *discordgo.Session) {
	appID := sess.State.User.ID
	devGuildID := os.Getenv("GUILD_ID")

	// Always register globally for users who authorize the app
	log.Println("Registering global application commands")
	createAllCommands(sess, appID, "")

	// If dev guild is set, also register there for instant updates during development
	if devGuildID != "" {
		log.Printf("Registering guild-scoped commands to dev guild %s for instant updates", devGuildID)
		createAllCommands(sess, appID, devGuildID)
	}
}

// createAllCommands registers all bot commands to the specified scope (empty string = global)
func createAllCommands(sess *discordgo.Session, appID, guildID string) {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	// ----------------------------------------
	// /analyse
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "analyse",
		Description: "Analyses an Image URL for inappropriate content",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
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
	}); err != nil {
		log.Printf("cannot create command analyse (%s): %v", scope, err)
	} else {
		log.Printf("created command: analyse (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /ping
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Pong!",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
	}); err != nil {
		log.Printf("cannot create command ping (%s): %v", scope, err)
	} else {
		log.Printf("created command: ping (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /help
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows a list of commands",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
	}); err != nil {
		log.Printf("cannot create command help (%s): %v", scope, err)
	} else {
		log.Printf("created command: help (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /thresholds [list | set | reset | history]
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "thresholds",
		Description: "Shows or modifies detection thresholds",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
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
	}); err != nil {
		log.Printf("cannot create command thresholds (%s): %v", scope, err)
	} else {
		log.Printf("created command: thresholds (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /ai
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ai",
		Description: "Checks an Image URL for AI usage",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "image_url",
			Description: "The Image URL to check",
			Required:    true,
		}},
	}); err != nil {
		log.Printf("cannot create command ai (%s): %v", scope, err)
	} else {
		log.Printf("created command: ai (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /reverse
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "reverse",
		Description: "Performs a reverse image search on an Image URL",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "image_url",
			Description: "The Image URL to check",
			Required:    true,
		}},
	}); err != nil {
		log.Printf("cannot create command reverse (%s): %v", scope, err)
	} else {
		log.Printf("created command: reverse (id=%s) [%s]", cmd.ID, scope)
	}

	// ----------------------------------------
	// /permissions <add | remove | list>
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "permissions",
		Description: "Manage roles allowed to use moderator-only commands",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild, // Only in guilds since it manages roles
		},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a moderator role allowed to use moderator-only commands",
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to add",
					Required:    true,
				}}},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "Remove a moderator role",
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to remove",
					Required:    true,
				}}},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List moderator roles allowed to use restricted commands",
			},
		},
	}); err != nil {
		log.Printf("cannot create command permissions (%s): %v", scope, err)
	} else {
		log.Printf("created command: permissions (id=%s) [%s]", cmd.ID, scope)
	}
}
