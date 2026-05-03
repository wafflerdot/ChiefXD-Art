package main

import (
	"ChiefXD-Art/commands"
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
)

// registerCommands deletes any existing commands and then creates fresh ones.
// Behaviour (dev toggle):
// - If env GUILD_ID is empty: manage GLOBAL commands only (delete/create global).
// - If env GUILD_ID is set: manage ONLY that GUILD'S commands (delete/create guild) and delete any GLOBAL commands to avoid duplicates in that guild.
func registerCommands(sess *discordgo.Session) {
	appID := sess.State.User.ID
	devGuildID := os.Getenv("GUILD_ID")

	if devGuildID == "" {
		// Global-only mode
		log.Println("Deleting existing global application commands")
		deleteAllCommands(sess, appID, "")

		log.Println("Registering global application commands")
		createAllCommands(sess, appID, "")
		return
	}

	// Dev/guild-only mode
	log.Printf("Deleting existing guild-scoped commands for dev guild %s", devGuildID)
	deleteAllCommands(sess, appID, devGuildID)

	// Also delete global commands to ensure they don't appear alongside guild-scoped ones in the dev guild
	log.Println("Deleting existing global application commands to avoid duplicates in dev guild")
	deleteAllCommands(sess, appID, "")

	log.Printf("Registering guild-scoped commands to dev guild %s for instant updates", devGuildID)
	createAllCommands(sess, appID, devGuildID)
}

// deleteAllCommands removes all application commands for the given scope
// Pass guildID = "" to target global commands; otherwise, the specified guild is targeted
func deleteAllCommands(sess *discordgo.Session, appID, guildID string) {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	// Fetch currently registered commands for the scope
	cmds, err := sess.ApplicationCommands(appID, guildID)
	if err != nil {
		log.Printf("failed to fetch existing commands (%s): %v", scope, err)
		return
	}
	if len(cmds) == 0 {
		log.Printf("no existing commands to delete (%s)", scope)
		return
	}

	deleted := 0
	for _, c := range cmds {
		if err := sess.ApplicationCommandDelete(appID, guildID, c.ID); err != nil {
			log.Printf("failed to delete command %s (%s): %v", c.Name, scope, err)
			continue
		}
		deleted++
	}
	log.Printf("deleted %d command(s) [%s]", deleted, scope)
}

// createAllCommands registers all bot commands to the specified scope (empty string = global)
func createAllCommands(sess *discordgo.Session, appID, guildID string) {
	// Register all commands from the commands package
	_ = commands.RegisterAnalyse(sess, appID, guildID)
	_ = commands.RegisterPing(sess, appID, guildID)
	_ = commands.RegisterHelp(sess, appID, guildID)
	_ = commands.RegisterAI(sess, appID, guildID)
	_ = commands.RegisterReverse(sess, appID, guildID)
	_ = commands.RegisterWaffle(sess, appID, guildID)
	if !dbLessMode {
		_ = commands.RegisterThresholds(sess, appID, guildID)
		_ = commands.RegisterPermissions(sess, appID, guildID)
		_ = commands.RegisterWhitelist(sess, appID, guildID)
	}
}
