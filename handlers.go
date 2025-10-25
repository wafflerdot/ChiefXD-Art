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

// registerHandlers wires all slash command handlers onto the session.
func registerHandlers(sess *discordgo.Session) {
	// Apply Rich Presence on READY
	sess.AddHandler(onReadySetPresence)

	// /permissions add|remove|list
	sess.AddHandler(handlePermissions)

	// /analyse
	sess.AddHandler(handleAnalyse)

	// /ai
	sess.AddHandler(handleAI)

	// /ping
	sess.AddHandler(handlePing)

	// /help
	sess.AddHandler(handleHelp)

	// /thresholds
	sess.AddHandler(handleThresholds)
}

// -------------------------
// Admin: /permissions
// -------------------------
func handlePermissions(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "permissions" {
		return
	}

	// Only owner or admins can manage permissions
	if !(IsOwner(i.Member.User.ID) || HasAdminContextPermission(i)) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You don't have permission to manage the whitelist.",
			},
		})
		return
	}

	// Defer to allow processing
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer permissions:", err)
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		msg := "Missing subcommand. Use add, remove or list."
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}

	sub := data.Options[0]
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
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
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
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
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
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})

	default:
		msg := "Unknown subcommand."
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}
}

// -------------------------
// /analyse
// -------------------------
func handleAnalyse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "analyse" {
		return
	}
	if !perms.IsAllowedForRestricted(i) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You don't have permission to use this command."}})
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
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You don't have permission to use this command."}})
		return
	}
	aiCommandHandlerBody(s, i)
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
		}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
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
			{Name: "/ai", Value: "Checks an Image URL for AI usage.\nArguments: `image_url` (required): The Image URL to check", Inline: false},
			{Name: "/analyse", Value: "Analyses an Image URL for inappropriate content.\nArguments:\n- `image_url` (required): The Image URL to analyse\n- `advanced` (optional): `true` shows detailed category and subcategory scores for nudity, offensive content and AI usage", Inline: false},
			{Name: "/help", Value: "Shows this message", Inline: false},
			{Name: "/permissions", Value: "Manage which roles can use moderator-only commands", Inline: false},
			{Name: "/ping", Value: "Displays the bot's response time", Inline: false},
			{Name: "/thresholds", Value: "Shows the current detection thresholds", Inline: false},
		}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

// -------------------------
// /thresholds
// -------------------------
func handleThresholds(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "thresholds" {
		return
	}
	if !perms.IsAllowedForRestricted(i) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "You don't have permission to use this command."}})
		return
	}

	data := i.ApplicationCommandData()
	// If no subcommand, show current thresholds
	if len(data.Options) == 0 {
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
			log.Println("failed to defer thresholds:", err)
			return
		}
		val := fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			NudityExplicitThreshold*100, NuditySuggestiveThreshold*100, OffensiveThreshold*100, AIGeneratedThreshold*100)
		embed := &discordgo.MessageEmbed{Title: "Detection Thresholds", Description: "Current thresholds to flag image as inappropriate", Color: 0x9C27B0,
			Fields: []*discordgo.MessageEmbedField{{Name: "Thresholds", Value: val, Inline: false}}, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// Has subcommand
	sub := data.Options[0]
	switch sub.Name {
	case "set":
		var name, valueStr string
		for _, opt := range sub.Options {
			if opt.Name == "name" {
				name = strings.TrimSpace(opt.StringValue())
			}
			if opt.Name == "value" {
				valueStr = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" || valueStr == "" {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Usage: /thresholds set <Name> <Value>"}})
			return
		}
		// Parse value: allow percent like 70% or 0.70
		val, err := parseThresholdValue(valueStr)
		if err != nil || val < 0 || val > 1 {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Value must be a number between 0.00 and 1.00, or a percentage like 70%"}})
			return
		}
		// Normalise name variants
		canonical, ok := canonicalThresholdName(name)
		if !ok {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Unknown threshold name. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated"}})
			return
		}
		if err := thresholdsStore.Set(perms, canonical, val); err != nil {
			log.Println("thresholds set error:", err)
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Failed to update threshold"}})
			return
		}
		msg := fmt.Sprintf("Set %s to %.2f%%", canonical, val*100)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})

	case "reset":
		var name string
		for _, opt := range sub.Options {
			if opt.Name == "name" {
				name = strings.TrimSpace(opt.StringValue())
			}
		}
		if name == "" {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Usage: /thresholds reset <Name|all>"}})
			return
		}
		if strings.EqualFold(name, "all") {
			if err := thresholdsStore.ResetAll(perms); err != nil {
				log.Println("thresholds reset all error:", err)
				_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{Content: "Failed to reset thresholds"}})
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Reset all thresholds to default"}})
			return
		}
		canonical, ok := canonicalThresholdName(name)
		if !ok {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Unknown threshold name. Use NuditySuggestive, NudityExplicit, Offensive, or AIGenerated"}})
			return
		}
		if err := thresholdsStore.ResetOne(perms, canonical); err != nil {
			log.Println("thresholds reset one error:", err)
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Failed to reset threshold"}})
			return
		}
		msg := fmt.Sprintf("Reset %s to default", canonical)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: msg}})
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
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Missing `image_url`."}})
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
			Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}
	// Standard
	a, err := AnalyseImageURL(imageURL)
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
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
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
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Missing `image_url`."}})
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}); err != nil {
		log.Println("failed to defer ai interaction:", err)
		return
	}
	analysis, err := AnalyseImageURLAIOnly(imageURL)
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
		Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: FooterText}}
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
