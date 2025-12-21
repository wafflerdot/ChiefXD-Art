package commands

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// RegisterWhitelist registers the /whitelist command definition
func RegisterWhitelist(s *discordgo.Session, appID, guildID string) error {
	scope := "global"
	if guildID != "" {
		scope = "guild " + guildID
	}

	cmd, err := s.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "whitelist",
		Description: "Manage whitelist (owner only)",
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
			discordgo.ApplicationIntegrationUserInstall,
		},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a user to the whitelist (owner only)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to whitelist (pick a user)",
						Required:    false,
					},
					{Type: discordgo.ApplicationCommandOptionString, Name: "user_id", Description: "Alternative: Discord User ID", Required: false},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "Remove a user from the whitelist (owner only)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to remove (pick a user)",
						Required:    false,
					},
					{Type: discordgo.ApplicationCommandOptionString, Name: "user_id", Description: "Alternative: Discord User ID", Required: false},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List all whitelisted users (owner only)",
			},
		},
	})
	if err != nil {
		log.Printf("cannot create command whitelist (%s): %v", scope, err)
		return err
	}
	log.Printf("created command: whitelist (id=%s) [%s]", cmd.ID, scope)
	return nil
}

// HandleWhitelist handles the /whitelist command interaction
func HandleWhitelist(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, isOwner func(string) bool, invokingUserID func(*discordgo.InteractionCreate) string, footerText, footerIconURL string) {
	if !isOwner(invokingUserID(i)) {
		_ = RespondEphemeral(s, i, "Only the bot owner can manage the whitelist")
		return
	}
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		_ = RespondEphemeral(s, i, "Missing subcommand. Use add, remove, or list")
		return
	}
	sub := data.Options[0]
	action := sub.Name

	// For add/remove, parse the user option first, then fall back to user_id string
	if action == "add" || action == "remove" {
		var targetID string
		var providedByID bool
		for _, opt := range sub.Options {
			if opt.Name == "user" && opt.Type == discordgo.ApplicationCommandOptionUser {
				if u := opt.UserValue(s); u != nil {
					targetID = u.ID
				}
			}
			if opt.Name == "user_id" && targetID == "" {
				val := strings.TrimSpace(opt.StringValue())
				if val != "" {
					targetID = val
					providedByID = true
				}
			}
		}
		if targetID == "" {
			_ = RespondEphemeral(s, i, "Please specify a user (user picker) or a numeric user ID")
			return
		}

		// Stricter validation for user_id path
		if providedByID {
			if !IsValidSnowflake(targetID) {
				_ = RespondEphemeral(s, i, "The provided user ID is not a valid Discord ID")
				return
			}
			// For add, attempt to fetch the user to ensure existence
			if action == "add" {
				if !CanFetchUser(s, targetID) {
					_ = RespondEphemeral(s, i, "Could not validate user ID. Please check the ID or use the user picker")
					return
				}
			}
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer whitelist interaction:", err)
			return
		}
		switch action {
		case "add":
			perms.AddWhitelistedUser(targetID)
			msg := fmt.Sprintf("Added <@%s> to the whitelist", targetID)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		case "remove":
			// For removal, numeric snowflake validation is sufficient when provided via user_id
			if providedByID && !IsValidSnowflake(targetID) {
				msg := "The provided user ID is not a valid Discord ID"
				_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
				return
			}
			perms.RemoveWhitelistedUser(targetID)
			msg := fmt.Sprintf("Removed <@%s> from the whitelist", targetID)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		}
		return
	}

	// list subcommand: with pagination if long
	if action == "list" {
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer whitelist list interaction:", err)
			return
		}
		ids := perms.ListWhitelistedUsers()
		if len(ids) == 0 {
			msg := "No users are currently whitelisted"
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		// Sort for stable paging
		sort.Strings(ids)
		page := 0
		pageSize := 10
		embed, comps := BuildWhitelistPageEmbed(s, ids, page, pageSize, footerText, footerIconURL)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &comps})
		return
	}

	_ = RespondEphemeral(s, i, "Unknown subcommand. Use add, remove, or list.")
}

// BuildWhitelistPageEmbed builds an embed page and action rows for pagination
func BuildWhitelistPageEmbed(s *discordgo.Session, ids []string, page, pageSize int, footerText, footerIconURL string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	total := len(ids)
	pages := (total + pageSize - 1) / pageSize
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	lines := make([]string, 0, end-start)
	for _, id := range ids[start:end] {
		name := id
		if s != nil && s.State != nil {
			if u, _ := s.User(id); u != nil && u.Username != "" {
				name = u.Username
			}
		}
		lines = append(lines, fmt.Sprintf("%s: <@%s>", name, id))
	}
	val := strings.Join(lines, "\n")
	title := fmt.Sprintf("DM Whitelist (%d/%d)", page+1, pages)
	embed := &discordgo.MessageEmbed{Title: title, Description: "Users allowed to use restricted commands in DMs", Color: 0x95A5A6,
		Fields: []*discordgo.MessageEmbedField{{Name: "Users", Value: val, Inline: false}}, Footer: &discordgo.MessageEmbedFooter{Text: footerText, IconURL: footerIconURL}}

	prevDisabled := page <= 0
	nextDisabled := page >= pages-1
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		&discordgo.Button{CustomID: fmt.Sprintf("wl_prev:%d:%d", page, pageSize), Label: "Prev", Style: discordgo.PrimaryButton, Disabled: prevDisabled},
		&discordgo.Button{CustomID: fmt.Sprintf("wl_next:%d:%d", page, pageSize), Label: "Next", Style: discordgo.PrimaryButton, Disabled: nextDisabled},
	}}
	return embed, []discordgo.MessageComponent{row}
}

// HandleWhitelistComponents handles component interactions for whitelist pagination
func HandleWhitelistComponents(s *discordgo.Session, i *discordgo.InteractionCreate, perms PermissionChecker, isOwner func(string) bool, invokingUserID func(*discordgo.InteractionCreate) string, footerText, footerIconURL string) {
	cid := i.MessageComponentData().CustomID
	if !strings.HasPrefix(cid, "wl_") {
		return
	}
	// Only owner can page whitelist
	if !isOwner(invokingUserID(i)) {
		_ = RespondEphemeral(s, i, "Only the owner can view the whitelist")
		return
	}
	parts := strings.Split(cid, ":")
	if len(parts) != 3 {
		return
	}
	action := parts[0] // wl_prev or wl_next
	curPage, _ := strconv.Atoi(parts[1])
	pageSize, _ := strconv.Atoi(parts[2])
	ids := perms.ListWhitelistedUsers()
	if len(ids) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{Content: "No users are currently whitelisted", Components: []discordgo.MessageComponent{}},
		})
		return
	}
	sort.Strings(ids)
	page := curPage
	if action == "wl_prev" && page > 0 {
		page = page - 1
	}
	if action == "wl_next" {
		pages := (len(ids) + pageSize - 1) / pageSize
		if page < pages-1 {
			page = page + 1
		}
	}
	embed, comps := BuildWhitelistPageEmbed(s, ids, page, pageSize, footerText, footerIconURL)
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: comps},
	})
}

// IsValidSnowflake checks basic Discord snowflake format: numeric string length 15..21
func IsValidSnowflake(id string) bool {
	if len(id) < 15 || len(id) > 21 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CanFetchUser attempts to fetch a user by ID; returns true if a user is returned
func CanFetchUser(s *discordgo.Session, id string) bool {
	if s == nil {
		return false
	}
	u, err := s.User(id)
	if err != nil || u == nil || u.ID == "" {
		return false
	}
	return true
}
