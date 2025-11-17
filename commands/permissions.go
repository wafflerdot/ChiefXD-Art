package commands

import (
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// RegisterPermissions registers the /permissions command definition
func RegisterPermissions(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "permissions",
		Description: "Manage roles allowed to use moderator-only commands",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild, // Only in guilds since it manages roles
		},
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall, // Only guild install
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
	})
	if err != nil {
		log.Printf("cannot create command permissions (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: permissions (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandlePermissions handles the /permissions command interaction
func HandlePermissions(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, isOwner func(string) bool, hasAdminPerm func(*discordgo.InteractionCreate) bool, invokingUserID func(*discordgo.InteractionCreate) string, formatRoleList func(*discordgo.Session, string, []string) string, footerText, footerIconURL string) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "permissions" {
		return
	}

	// Only owner or admins can manage permissions
	if !(isOwner(invokingUserID(i)) || hasAdminPerm(i)) {
		_ = RespondEphemeral(s, i, "You don't have permission to manage the whitelist.")
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		_ = RespondEphemeral(s, i, "Missing subcommand. Use add, remove or list.")
		return
	}

	sub := data.Options[0]
	if sub.Name != "add" && sub.Name != "remove" && sub.Name != "list" {
		_ = RespondEphemeral(s, i, "Unknown subcommand.")
		return
	}

	// Defer to allow processing for valid subcommands
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Println("failed to defer permissions:", err)
		return
	}

	switch sub.Name {
	case "add":
		var roleID string
		for _, opt := range sub.Options {
			if opt.Name == "role" {
				roleID = opt.RoleValue(s, i.GuildID).ID
			}
		}
		if roleID == "" {
			msg := "Missing role."
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		perms.AddRole(i.GuildID, roleID)
		list := perms.ListRoles(i.GuildID)
		val := formatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions Updated",
			Description: "Added role <@&" + roleID + ">",
			Color:       0x2ECC71,
			Fields: []*discordgo.MessageEmbedField{{
				Name:   "Allowed Roles",
				Value:  val,
				Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})

	case "remove":
		var roleID string
		for _, opt := range sub.Options {
			if opt.Name == "role" {
				roleID = opt.RoleValue(s, i.GuildID).ID
			}
		}
		if roleID == "" {
			msg := "Missing role."
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		perms.RemoveRole(i.GuildID, roleID)
		list := perms.ListRoles(i.GuildID)
		val := formatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions Updated",
			Description: "Removed role <@&" + roleID + ">",
			Color:       0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{{
				Name:  "Allowed Roles",
				Value: val, Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})

	case "list":
		list := perms.ListRoles(i.GuildID)
		val := formatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions",
			Description: "Roles allowed to use restricted commands",
			Color:       0x3498DB,
			Fields: []*discordgo.MessageEmbedField{{
				Name:  "Allowed Roles",
				Value: val, Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
	}
}

// FormatRoleList formats a list of role IDs as mentions
func FormatRoleList(s *discordgo.Session, guildID string, roleIDs []string) string {
	if len(roleIDs) == 0 {
		return "No roles configured. Only the bot owner and server admins can use restricted commands."
	}
	var mentions []string
	for _, id := range roleIDs {
		mentions = append(mentions, "<@&"+id+">")
	}
	return strings.Join(mentions, ", ")
}
