package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

const (
	FooterText = "Bot created by wafflerdot"
)

var httpServer *http.Server

func startHTTPServer() {
	// Minimal HTTP server for Google Cloud Run health checks
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
}

func main() {
	_ = godotenv.Load()

	// Discord Bot
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN must be set in environment variables")
	}

	// Start HTTP server
	startHTTPServer()

	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if sess == nil {
			return
		}
		if err := sess.Close(); err != nil {
			log.Println("failed to close Discord session:", err)
		}
	}()

	// Apply Rich Presence on READY
	sess.AddHandler(onReadySetPresence)

	// Command handler: /analyse <image_url> [advanced]
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Only respond to application command interactions.
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "analyse" {
			return
		}

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
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Missing `image_url`.",
				},
			})
			return
		}

		// Acknowledge immediately to avoid the 3s timeout, then edit with results
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Println("failed to defer interaction:", err)
			return
		}

		// If advanced is requested, use the advanced analysis and embed
		if advanced {
			advAnalysis, err := AnalyseImageURLAdvanced(imageURL)
			if err != nil {
				msg := fmt.Sprintf("Analysis failed: %v", err)
				_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &msg,
				})
				return
			}

			// Helper to format a category map into a multiline percentage list sorted by score description
			formatScores := func(title string, m map[string]float64) *discordgo.MessageEmbedField {
				if len(m) == 0 {
					return &discordgo.MessageEmbedField{Name: title, Value: "none", Inline: false}
				}
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
				var builder strings.Builder
				for _, k := range keys {
					if _, err := fmt.Fprintf(&builder, "%s: %.0f%%\n", k, m[k]*100); err != nil {
						log.Println("failed to write score to buffer:", err)
					}
				}
				val := strings.TrimRight(builder.String(), "\n")
				return &discordgo.MessageEmbedField{Name: title, Value: val, Inline: false}
			}

			fields := make([]*discordgo.MessageEmbedField, 0, 6)
			if nudity, ok := advAnalysis.Categories["nudity"]; ok {
				fields = append(fields, formatScores("Nudity", nudity))
			}
			if offensive, ok := advAnalysis.Categories["offensive"]; ok {
				fields = append(fields, formatScores("Offensive Content", offensive))
			}
			if typ, ok := advAnalysis.Categories["type"]; ok {
				fields = append(fields, formatScores("AI Usage", typ))
			}

			embed := &discordgo.MessageEmbed{
				Title:       "Image Analysis (Advanced)",
				Description: fmt.Sprintf("Analysis results for: %s", imageURL),
				Color:       0x4CAF50,
				Fields:      fields,
				Footer: &discordgo.MessageEmbedFooter{
					Text: FooterText,
				},
			}

			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds: &[]*discordgo.MessageEmbed{embed},
			})
			return
		}

		// Standard analysis
		analysis, err := AnalyseImageURL(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}

		fields := []*discordgo.MessageEmbedField{
			{
				Name:   "Safe Image",
				Value:  fmt.Sprintf("%t", analysis.Allowed),
				Inline: true,
			},
			{
				Name: "Results",
				Value: fmt.Sprintf(
					"Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
					analysis.Scores.NudityExplicit*100,
					analysis.Scores.NuditySuggestive*100,
					analysis.Scores.Offensive*100,
					analysis.Scores.AIGenerated*100,
				),
				Inline: false,
			},
		}

		embed := &discordgo.MessageEmbed{
			Title:       "Image Analysis",
			Description: fmt.Sprintf("Analysis results for: %s", imageURL),
			Color:       0x00BFA5,
			Fields:      fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: FooterText,
			},
		}

		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	})

	// Command handler: /ai <image_url>
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "ai" {
			return
		}

		var imageURL string
		for _, opt := range i.ApplicationCommandData().Options {
			if opt.Name == "image_url" {
				imageURL = opt.StringValue()
			}
		}
		if imageURL == "" {
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Missing `image_url`.",
				},
			})
			return
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
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
		embed := &discordgo.MessageEmbed{
			Title:       "AI Usage Check",
			Description: fmt.Sprintf("Analysis results for: %s", imageURL),
			Color:       0x3F51B5,
			Fields:      fields,
			Footer:      &discordgo.MessageEmbedFooter{Text: FooterText},
		}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
	})

	// Command handler: /ping
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "ping" {
			return
		}

		start := time.Now()
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Println("failed to defer ping:", err)
			return
		}

		rtt := time.Since(start)
		gw := s.HeartbeatLatency()

		embed := &discordgo.MessageEmbed{
			Title: "Pong!",
			Color: 0xFFC107,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Response time", Value: fmt.Sprintf("%d ms", rtt.Milliseconds()), Inline: true},
				{Name: "Gateway latency", Value: fmt.Sprintf("%d ms", gw.Milliseconds()), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText},
		}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	})

	// Command handler: /help
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "help" {
			return
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Println("failed to defer help:", err)
			return
		}

		embed := &discordgo.MessageEmbed{
			Title:       "Help",
			Description: "Available commands",
			Color:       0x5865F2,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "/analyse",
					Value:  "Analyses an Image URL for inappropriate content.\nArguments:\n- `image_url` (required): The Image URL to analyse\n- `advanced` (optional): `true` shows detailed category and subcategory scores for nudity, offensive content and AI usage",
					Inline: false,
				},
				{
					Name:   "/ping",
					Value:  "Displays the bot's response time",
					Inline: false,
				},
				{
					Name:   "/help",
					Value:  "Shows this message",
					Inline: false,
				},
				{
					Name:   "/thresholds",
					Value:  "Shows the current detection thresholds",
					Inline: false,
				},
				{
					Name:   "/ai",
					Value:  "Checks an Image URL for AI usage.\nArguments: `image_url` (required): The Image URL to check",
					Inline: false,
				},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText},
		}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	})

	// Command handler: /thresholds
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "thresholds" {
			return
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Println("failed to defer thresholds:", err)
			return
		}

		// Build a readable thresholds overview. Constants come from analysis.go.
		val := fmt.Sprintf(
			"Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%",
			NudityExplicitThreshold*100,
			NuditySuggestiveThreshold*100,
			OffensiveThreshold*100,
			AIGeneratedThreshold*100,
		)
		embed := &discordgo.MessageEmbed{
			Title:       "Detection Thresholds",
			Description: "Current thresholds to flag image as inappropriate",
			Color:       0x9C27B0,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Thresholds", Value: val, Inline: false},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: FooterText},
		}
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	})

	// Open session before registering commands so s.State.User is populated
	if err := sess.Open(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bot is now online!")

	// Command registration scope toggle:
	// If GUILD_ID is set (non-empty), register commands only for that guild (instant propagation).
	// If GUILD_ID is empty, register commands globally (may take up to ~1 hour to propagate).
	appID := sess.State.User.ID
	guildID := os.Getenv("GUILD_ID")
	if guildID == "" {
		log.Println("Registering global application commands (GUILD_ID not set)")
	} else {
		log.Printf("Registering guild-scoped application commands to guild %s", guildID)
	}

	// Register the /analyse command
	cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "analyse",
		Description: "Analyses an Image URL for inappropriate content",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "image_url",
				Description: "The Image URL to analyse",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "advanced",
				Description: "Advanced mode, shows more detailed results",
				Required:    false,
			},
		},
	})
	if err != nil {
		log.Fatalf("cannot create command analyse: %v", err)
	}
	log.Printf("created command: %s (id=%s)", cmd.Name, cmd.ID)

	// Register /ping
	cmdPing, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Pong!",
	})
	if err != nil {
		log.Fatalf("cannot create command ping: %v", err)
	}
	log.Printf("created command: %s (id=%s)", cmdPing.Name, cmdPing.ID)

	// Register /help
	cmdHelp, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows a list of commands",
	})
	if err != nil {
		log.Fatalf("cannot create command help: %v", err)
	}
	log.Printf("created command: %s (id=%s)", cmdHelp.Name, cmdHelp.ID)

	// Register /thresholds
	cmdThresholds, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "thresholds",
		Description: "Shows the current detection thresholds",
	})
	if err != nil {
		log.Fatalf("cannot create command thresholds: %v", err)
	}
	log.Printf("created command: %s (id=%s)", cmdThresholds.Name, cmdThresholds.ID)

	// Register /ai
	cmdAI, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ai",
		Description: "Checks an Image URL for AI usage",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString,
				Name:        "image_url",
				Description: "The Image URL to check",
				Required:    true,
			},
		},
	})
	if err != nil {
		log.Fatalf("cannot create command ai: %v", err)
	}
	log.Printf("created command: %s (id=%s)", cmdAI.Name, cmdAI.ID)

	// Debug: list commands in the chosen scope to verify what Discord has stored
	go func() {
		// Small delay to give Discord a moment to process creations.
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

	// Wait for exit signals
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Graceful shutdown for HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(ctx)
	}
}
