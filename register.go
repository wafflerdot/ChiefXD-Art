package main

import (
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
)

// registerCommands creates the slash commands either globally or guild-scoped.
func registerCommands(sess *discordgo.Session) {
	appID := sess.State.User.ID
	guildID := os.Getenv("GUILD_ID")

	if guildID == "" {
		log.Println("Registering global application commands (GUILD_ID not set)")
	} else {
		log.Printf("Registering guild-scoped application commands to guild %s", guildID)
	}

	// ----------------------------------------
	// /analyse
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "analyse",
		Description: "Analyses an Image URL for inappropriate content",
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "image_url",
			Description: "The Image URL to analyse",
			Required:    true},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "advanced",
				Description: "Advanced mode, shows more detailed results",
				Required:    false},
		},
	}); err != nil {
		log.Fatalf("cannot create command analyse: %v", err)
	} else {
		log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)
	}

	// ----------------------------------------
	// /ping
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Pong!",
	}); err != nil {
		log.Fatalf("cannot create command ping: %v", err)
	} else {
		log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)
	}

	// ----------------------------------------
	// /help
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows a list of commands",
	}); err != nil {
		log.Fatalf("cannot create command help: %v", err)
	} else {
		log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)
	}

	// ----------------------------------------
	// /thresholds (list | set | reset | history)
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "thresholds",
		Description: "Shows or modifies detection thresholds",
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
					{Type: discordgo.ApplicationCommandOptionString, Name: "threshold", Description: "NuditySuggestive, NudityExplicit, Offensive, AIGenerated", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "value", Description: "Decimal (0.00-1.00) or percentage (0-100%)", Required: true},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset",
				Description: "Reset thresholds to default (one or all)",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "threshold", Description: "NuditySuggestive, NudityExplicit, Offensive, AIGenerated, or all", Required: true},
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
				}},
		},
	}); err != nil {
		log.Fatalf("cannot create command thresholds: %v", err)
	} else {
		log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)
	}

	// ----------------------------------------
	// /ai
	// ----------------------------------------
	if cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ai",
		Description: "Checks an Image URL for AI usage",
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "image_url",
			Description: "The Image URL to check",
			Required:    true},
		},
	}); err != nil {
		log.Fatalf("cannot create command ai: %v", err)
	} else {
		log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)
	}

	// ----------------------------------------
	// /permissions <add | remove | list>
	// ----------------------------------------
	if _, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "permissions",
		Description: "Manage roles allowed to use moderator-only commands",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a moderator role allowed to use moderator-only commands",
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to add",
					Required:    true}}},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "Remove a moderator role",
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Role to remove",
					Required:    true}}},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List moderator roles allowed to use restricted commands"},
		},
	}); err != nil {
		log.Fatalf("cannot create command permissions: %v", err)
	}

	// ----------------------------------------
	// Debug list stored commands for the chosen scope
	// ----------------------------------------
	go func() {
		time.Sleep(2 * time.Second)

		listScope := guildID
		scopeLabel := "global"
		if guildID != "" {
			scopeLabel = "guild"
		}

		cmds, err := sess.ApplicationCommands(appID, listScope)
		if err != nil {
			log.Printf("failed to list %s application commands: %v", scopeLabel, err)
			return
		}
		for _, c := range cmds {
			log.Printf("discord stored %s command: name=%s id=%s", scopeLabel, c.Name, c.ID)
		}
	}()
}
