package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// respondEphemeral sends an ephemeral message visible only to the invoking user
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// registerHandlers wires all slash command handlers onto the session
func registerHandlers(sess *discordgo.Session) {
	// Apply Rich Presence on READY
	sess.AddHandler(onReadySetPresence)

	// Register commands when bot joins a new guild
	sess.AddHandler(onGuildCreate)

	// /permissions <add|remove|list>
	sess.AddHandler(handlePermissions)

	// /analyse <image_url> [advanced]
	sess.AddHandler(handleAnalyse)

	// /ai <image_url>
	sess.AddHandler(handleAI)

	// /ping
	sess.AddHandler(handlePing)

	// /help
	sess.AddHandler(handleHelp)

	// /thresholds [list|history|set|reset]
	sess.AddHandler(handleThresholds)

	// /reverse <image_url>
	sess.AddHandler(handleReverse)

	// /whitelist add|remove (owner only)
	sess.AddHandler(handleWhitelist)

	// Component interactions (pagination)
	sess.AddHandler(handleComponents)

	// owner-only waffle
	sess.AddHandler(handleWaffle)
}

// -------------------------
// Guild Join Handler
// -------------------------
// onGuildCreate is triggered when the bot joins a new guild or when the bot starts up
// and syncs all guilds. We register commands to the guild for instant availability.
func onGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	log.Printf("Bot joined or synced with guild: %s (%s)", event.Guild.Name, event.Guild.ID)

	// Register all commands to this guild for instant availability
	go func() {
		appID := s.State.User.ID
		createAllCommands(s, appID, event.Guild.ID)
		log.Printf("Commands registered for guild: %s", event.Guild.ID)
	}()
}

// -------------------------
// Admin: /permissions
// -------------------------
func handlePermissions(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "permissions" {
		return
	}

	// Only owner or admins can manage permissions
	if !(IsOwner(invokingUserID(i)) || HasAdminContextPermission(i)) {
		_ = respondEphemeral(s, i, "You don't have permission to manage the whitelist.")
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		_ = respondEphemeral(s, i, "Missing subcommand. Use add, remove or list.")
		return
	}

	sub := data.Options[0]
	if sub.Name != "add" && sub.Name != "remove" && sub.Name != "list" {
		_ = respondEphemeral(s, i, "Unknown subcommand.")
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
		val := FormatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions Updated",
			Description: "Added role <@&" + roleID + ">",
			Color:       0x2ECC71,
			Fields: []*discordgo.MessageEmbedField{{
				Name:   "Allowed Roles",
				Value:  val,
				Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
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
		val := FormatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions Updated",
			Description: "Removed role <@&" + roleID + ">",
			Color:       0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{{
				Name:  "Allowed Roles",
				Value: val, Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})

	case "list":
		list := perms.ListRoles(i.GuildID)
		val := FormatRoleList(s, i.GuildID, list)
		embed := &discordgo.MessageEmbed{
			Title:       "Permissions",
			Description: "Roles allowed to use restricted commands",
			Color:       0x3498DB,
			Fields: []*discordgo.MessageEmbedField{{
				Name:  "Allowed Roles",
				Value: val, Inline: false}},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
	}
}

// -------------------------
// /whitelist (owner only)
// -------------------------
func handleWhitelist(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "whitelist" {
		return
	}
	if !IsOwner(invokingUserID(i)) {
		_ = respondEphemeral(s, i, "Only the bot owner can manage the whitelist")
		return
	}
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		_ = respondEphemeral(s, i, "Missing subcommand. Use add, remove, or list")
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
			_ = respondEphemeral(s, i, "Please specify a user (user picker) or a numeric user ID")
			return
		}

		// Stricter validation for user_id path
		if providedByID {
			if !isValidSnowflake(targetID) {
				_ = respondEphemeral(s, i, "The provided user ID is not a valid Discord ID")
				return
			}
			// For add, attempt to fetch the user to ensure existence
			if action == "add" {
				if !canFetchUser(s, targetID) {
					_ = respondEphemeral(s, i, "Could not validate user ID. Please check the ID or use the user picker")
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
			if providedByID && !isValidSnowflake(targetID) {
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
		embed, comps := buildWhitelistPageEmbed(s, ids, page, pageSize)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &comps})
		return
	}

	_ = respondEphemeral(s, i, "Unknown subcommand. Use add, remove, or list.")
}

// buildWhitelistPageEmbed builds an embed page and action rows for pagination
func buildWhitelistPageEmbed(s *discordgo.Session, ids []string, page, pageSize int) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
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
		Fields: []*discordgo.MessageEmbedField{{Name: "Users", Value: val, Inline: false}}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}

	prevDisabled := page <= 0
	nextDisabled := page >= pages-1
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		&discordgo.Button{CustomID: fmt.Sprintf("wl_prev:%d:%d", page, pageSize), Label: "Prev", Style: discordgo.PrimaryButton, Disabled: prevDisabled},
		&discordgo.Button{CustomID: fmt.Sprintf("wl_next:%d:%d", page, pageSize), Label: "Next", Style: discordgo.PrimaryButton, Disabled: nextDisabled},
	}}
	return embed, []discordgo.MessageComponent{row}
}

// handle component interactions for whitelist pagination
func handleComponents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent || i.Message == nil || i.Message.Components == nil {
		return
	}
	cid := i.MessageComponentData().CustomID
	if !strings.HasPrefix(cid, "wl_") {
		return
	}
	// Only owner can page whitelist
	if !IsOwner(invokingUserID(i)) {
		_ = respondEphemeral(s, i, "Only the owner can view the whitelist")
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
	embed, comps := buildWhitelistPageEmbed(s, ids, page, pageSize)
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: comps},
	})
}

// -------------------------
// /analyse
// -------------------------
func handleAnalyse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "analyse" {
		return
	}
	if !perms.IsAllowedForRestricted(i) {
		_ = respondEphemeral(s, i, "You don't have permission to use this command.")
		return
	}
	analyseCommandHandlerBody(s, i)
}

// -------------------------
// /ai
// -------------------------
func handleAI(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "ai" {
		return
	}
	if !perms.IsAllowedForRestricted(i) {
		_ = respondEphemeral(s, i, "You don't have permission to use this command.")
		return
	}
	aiCommandHandlerBody(s, i)
}

// -------------------------
// /reverse
// -------------------------
func handleReverse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "reverse" {
		return
	}
	// Make this command owner-only
	if !IsOwner(invokingUserID(i)) {
		_ = respondEphemeral(s, i, "Only the bot owner can use this command.")
		return
	}
	var imageURL string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "image_url" {
			imageURL = opt.StringValue()
		}
	}
	if imageURL == "" {
		_ = respondEphemeral(s, i, "Missing `image_url`.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer reverse interaction:", err)
		return
	}
	res, err := ReverseLookup(imageURL)
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
	embed := &discordgo.MessageEmbed{Title: "Reverse Image Search", Description: desc, Color: color, Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

// -------------------------
// /ping
// -------------------------
func handlePing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "ping" {
		return
	}
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
		}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

// -------------------------
// /help
// -------------------------
func handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "help" {
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer help:", err)
		return
	}
	embed := &discordgo.MessageEmbed{Title: "Help", Description: "Available commands", Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "/ai", Value: "Checks an Image URL for AI usage\nArguments: `image_url` (required)", Inline: false},
			{Name: "/analyse", Value: "Analyses an Image URL for inappropriate content\nArguments:\n- `image_url` (required)\n- `advanced` (optional): `true` shows detailed category and subcategory scores", Inline: false},
			{Name: "/help", Value: "Shows this message", Inline: false},
			{Name: "/permissions", Value: "Manage which roles can use moderator-only commands (owner/admin only)", Inline: false},
			{Name: "/ping", Value: "Displays the bot's response time", Inline: false},
			{Name: "/reverse", Value: "Performs a reverse image search on an Image URL\nArguments: `image_url` (required)", Inline: false},
			{Name: "/thresholds", Value: "Shows or modifies detection thresholds\nSubcommands:\n- `list`: View current thresholds\n- `history [limit] [threshold]`: View recent changes\n- `set <Threshold> <Value>`: Modify a detection threshold (owner/admin only)\n- `reset <Threshold|all>`: Resets a threshold to its default value (owner/admin only)", Inline: false},
			{Name: "/whitelist", Value: "Manage DM whitelist (owner only). Subcommands: `add <user>`, `remove <user>`, `list`", Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

// -------------------------
// /thresholds
// -------------------------
func handleThresholds(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "thresholds" {
		return
	}

	data := i.ApplicationCommandData()
	guildID := i.GuildID

	// If no subcommand or list => view only (allowed roles can view)
	if len(data.Options) == 0 || data.Options[0].Name == "list" {
		if !(HasAdminContextPermission(i) || perms.IsAllowedForRestricted(i)) {
			_ = respondEphemeral(s, i, "You don't have permission to view thresholds.")
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer thresholds:", err)
			return
		}
		ns, ne, off, ai := thresholdsStore.GetGuildThresholds(perms, guildID)
		val := fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			ne*100, ns*100, off*100, ai*100)
		embed := &discordgo.MessageEmbed{Title: "Detection Thresholds", Description: "Current thresholds to flag image", Color: 0x9C27B0,
			Fields: []*discordgo.MessageEmbedField{{Name: "Thresholds", Value: val, Inline: false}}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// History: view-only (allowed roles or admins)
	if data.Options[0].Name == "history" {
		if !(HasAdminContextPermission(i) || perms.IsAllowedForRestricted(i)) {
			_ = respondEphemeral(s, i, "You don't have permission to view threshold history.")
			return
		}
		limit := 10
		var nameFilter string
		for _, opt := range data.Options[0].Options {
			if opt.Name == "limit" {
				limit = int(opt.IntValue())
			}
			if opt.Name == "threshold" {
				nameFilter = strings.TrimSpace(opt.StringValue())
			}
		}
		var (
			changes []ThresholdChange
			err     error
		)
		if nameFilter != "" {
			canonical, ok := canonicalThresholdName(nameFilter)
			if !ok {
				_ = respondEphemeral(s, i, "Unknown threshold filter. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
				return
			}
			changes, err = thresholdsStore.HistoryFilteredForGuild(perms, guildID, canonical, limit)
		} else {
			changes, err = thresholdsStore.HistoryForGuild(perms, guildID, limit)
		}
		if err != nil {
			log.Println("thresholds history error:", err)
			_ = respondEphemeral(s, i, "Failed to fetch history")
			return
		}
		if len(changes) == 0 {
			_ = respondEphemeral(s, i, "No history available.")
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer thresholds history:", err)
			return
		}
		fields := make([]*discordgo.MessageEmbedField, 0, len(changes))
		for _, c := range changes {
			user := "unknown"
			if c.UserID.Valid {
				user = "<@" + c.UserID.String + ">"
			}
			old := "n/a"
			if c.OldValue.Valid {
				old = fmt.Sprintf("%.2f%%", c.OldValue.Float64*100)
			}
			val := fmt.Sprintf("%s\nOld: %s → New: %.2f%%\nBy: %s\nAt: %s",
				c.Name, old, c.NewValue*100, user, c.Created.Format(time.RFC3339))
			fields = append(fields, &discordgo.MessageEmbedField{Name: "Change", Value: val, Inline: false})
		}
		embed := &discordgo.MessageEmbed{Title: "Thresholds History", Color: 0x8E44AD, Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// set/reset require owner/admin privileges
	if !(IsOwner(invokingUserID(i)) || HasAdminContextPermission(i)) {
		_ = respondEphemeral(s, i, "Only server admins or the owner can modify thresholds.")
		return
	}

	sub := data.Options[0]
	switch sub.Name {
	case "set":
		var name, valueStr string

		// Parse value: allow percent like 70% or 0.70
		for _, opt := range sub.Options {
			if opt.Name == "threshold" {
				name = strings.TrimSpace(opt.StringValue())
			}
			if opt.Name == "value" {
				// Normalise name variants
				valueStr = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" || valueStr == "" {
			_ = respondEphemeral(s, i, "Usage: /thresholds set <Name> <Value>")
			return
		}
		val, err := parseThresholdValue(valueStr)
		if err != nil || val < 0 || val > 1 {
			_ = respondEphemeral(s, i, "Value must be a decimal between 0.00 and 1.00, or a percentage like 70%")
			return
		}
		canonical, ok := canonicalThresholdName(name)
		if !ok {
			_ = respondEphemeral(s, i, "Unknown threshold. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
			return
		}
		oldNS, oldNE, oldOff, oldAI := thresholdsStore.GetGuildThresholds(perms, guildID)
		oldMap := map[string]float64{"NuditySuggestive": oldNS, "NudityExplicit": oldNE, "Offensive": oldOff, "AIGenerated": oldAI}
		if err := thresholdsStore.SetGuild(perms, guildID, canonical, val); err != nil {
			log.Println("thresholds set guild error:", err)
			_ = respondEphemeral(s, i, "Failed to update threshold")
			return
		}
		_ = thresholdsStore.LogChange(perms, canonical, oldMap[canonical], val, invokingUserID(i), guildID)
		msg := fmt.Sprintf("Set %s to %.2f%%", canonical, val*100)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})

	case "reset":
		var name string
		for _, opt := range sub.Options {
			if opt.Name == "threshold" {
				name = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" {
			_ = respondEphemeral(s, i, "Usage: /thresholds reset <Name|all>")
			return
		}
		if strings.EqualFold(name, "all") {
			oldNS, oldNE, oldOff, oldAI := thresholdsStore.GetGuildThresholds(perms, guildID)
			if err := thresholdsStore.ResetAllGuild(perms, guildID); err != nil {
				log.Println("thresholds reset all guild error:", err)
				_ = respondEphemeral(s, i, "Failed to reset thresholds")
				return
			}
			_ = thresholdsStore.LogChange(perms, "NuditySuggestive", oldNS, DefaultNuditySuggestiveThreshold, invokingUserID(i), guildID)
			_ = thresholdsStore.LogChange(perms, "NudityExplicit", oldNE, DefaultNudityExplicitThreshold, invokingUserID(i), guildID)
			_ = thresholdsStore.LogChange(perms, "Offensive", oldOff, DefaultOffensiveThreshold, invokingUserID(i), guildID)
			_ = thresholdsStore.LogChange(perms, "AIGenerated", oldAI, DefaultAIGeneratedThreshold, invokingUserID(i), guildID)
			msg := "Reset all thresholds to default"
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: msg}})
			return
		}
		canonical, ok := canonicalThresholdName(name)
		if !ok {
			_ = respondEphemeral(s, i, "Unknown threshold. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated")
			return
		}
		oldNS, oldNE, oldOff, oldAI := thresholdsStore.GetGuildThresholds(perms, guildID)
		oldMap := map[string]float64{"NuditySuggestive": oldNS, "NudityExplicit": oldNE, "Offensive": oldOff, "AIGenerated": oldAI}
		if err := thresholdsStore.ResetOneGuild(perms, guildID, canonical); err != nil {
			log.Println("thresholds reset one guild error:", err)
			_ = respondEphemeral(s, i, "Failed to reset threshold")
			return
		}
		// after reset, new value equals built-in default
		_ = thresholdsStore.LogChange(perms, canonical, oldMap[canonical], defaultThresholdValue(canonical), invokingUserID(i), guildID)
		msg := fmt.Sprintf("Reset %s to default", canonical)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})
	}
}

// -------------------------
// /waffle <message>
// -------------------------
func handleWaffle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "waffle" {
		return
	}
	if !IsOwner(invokingUserID(i)) {
		_ = respondEphemeral(s, i, "Only the bot owner can use this command")
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
		_ = respondEphemeral(s, i, "Please provide a message")
		return
	}

	// Acknowledge ephemerally, then send the plain message to the channel
	if err := respondEphemeral(s, i, "Message sent."); err != nil {
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

// -------------------------
// Command bodies (helpers)
// -------------------------
func analyseCommandHandlerBody(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Extract options
	var (
		imageURL string
		advanced bool
	)
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "image_url":
			imageURL = opt.StringValue()
		case "advanced":
			advanced = opt.BoolValue()
		}
	}
	if imageURL == "" {
		_ = respondEphemeral(s, i, "Missing `image_url`.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer interaction:", err)
		return
	}
	if advanced {
		aa, err := AnalyseImageURLAdvanced(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		formatScores := func(title string, m map[string]float64) *discordgo.MessageEmbedField {
			if len(m) == 0 {
				return &discordgo.MessageEmbedField{Name: title, Value: "none", Inline: false}
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
			var b strings.Builder
			for _, k := range keys {
				_, _ = fmt.Fprintf(&b, "%s: %.0f%%\n", k, m[k]*100)
			}
			val := strings.TrimRight(b.String(), "\n")
			return &discordgo.MessageEmbedField{Name: title, Value: val, Inline: false}
		}
		fields := make([]*discordgo.MessageEmbedField, 0, 6)
		if nudity, ok := aa.Categories["nudity"]; ok {
			fields = append(fields, formatScores("Nudity", nudity))
		}
		if offensive, ok := aa.Categories["offensive"]; ok {
			fields = append(fields, formatScores("Offensive Content", offensive))
		}
		if typ, ok := aa.Categories["type"]; ok {
			fields = append(fields, formatScores("AI Usage", typ))
		}
		embed := &discordgo.MessageEmbed{Title: "Image Analysis (Advanced)", Description: fmt.Sprintf("Analysis results for: %s", imageURL), Color: 0x4CAF50,
			Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}
	// Standard
	a, err := AnalyseImageURL(i.GuildID, imageURL)
	if err != nil {
		msg := fmt.Sprintf("Analysis failed: %v", err)
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "Safe Image", Value: fmt.Sprintf("%t", a.Allowed), Inline: true},
		{Name: "Results", Value: fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			a.Scores.NudityExplicit*100, a.Scores.NuditySuggestive*100, a.Scores.Offensive*100, a.Scores.AIGenerated*100), Inline: false},
	}
	embed := &discordgo.MessageEmbed{Title: "Image Analysis", Description: fmt.Sprintf("Analysis results for: %s", imageURL), Color: 0x00BFA5,
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

func aiCommandHandlerBody(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var imageURL string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "image_url" {
			imageURL = opt.StringValue()
		}
	}
	if imageURL == "" {
		_ = respondEphemeral(s, i, "Missing `image_url`.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer ai interaction:", err)
		return
	}
	analysis, err := AnalyseImageURLAIOnly(i.GuildID, imageURL)
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
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText, IconURL: FooterIconURL}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

func parseThresholdValue(in string) (float64, error) {
	s := strings.TrimSpace(in)
	if strings.HasSuffix(s, "%") {
		p := strings.TrimSuffix(s, "%")
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, err
		}
		return f / 100.0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func canonicalThresholdName(in string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(in))
	switch s {
	case "nuditysuggestive", "suggestive", "nudity_suggestive":
		return "NuditySuggestive", true
	case "nudityexplicit", "explicit", "nudity_explicit":
		return "NudityExplicit", true
	case "offensive", "offensive_symbols", "offensivesymbols":
		return "Offensive", true
	case "aigenerated", "ai", "genai", "ai_generated":
		return "AIGenerated", true
	default:
		return "", false
	}
}

// isValidSnowflake checks basic Discord snowflake format: numeric string length 15..21
func isValidSnowflake(id string) bool {
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

// canFetchUser attempts to fetch a user by ID; returns true if a user is returned
func canFetchUser(s *discordgo.Session, id string) bool {
	if s == nil {
		return false
	}
	u, err := s.User(id)
	if err != nil || u == nil || u.ID == "" {
		return false
	}
	return true
}
