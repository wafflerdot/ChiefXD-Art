// go
// file: `main.go`
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

var httpSrv *http.Server

func startHTTPServer() {
	// Minimal HTTP server for Cloud Run health/readiness.
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
	httpSrv = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	// Start Cloud Run HTTP server (health/readiness).
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

	// Apply Rich Presence on READY.
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

		// Acknowledge immediately to avoid the 3s timeout, then edit with results.
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Println("failed to defer interaction:", err)
			return
		}

		// If advanced is requested, use the advanced analysis and embed.
		if advanced {
			aa, err := AnalyseImageURLAdvanced(imageURL)
			if err != nil {
				msg := fmt.Sprintf("Analysis failed: %v", err)
				_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &msg,
				})
				return
			}

			// Helper to format a category map into a multiline percentage list sorted by score desc.
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
					if _, err := fmt.Fprintf(&b, "%s: %.0f%%\n", k, m[k]*100); err != nil {
						log.Println("failed to write score to buffer:", err)
					}
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
		a, err := AnalyseImageURL(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}

		fields := []*discordgo.MessageEmbedField{
			{Name: "Safe Image", Value: fmt.Sprintf("%t", a.Allowed), Inline: true},
			{Name: "Results", Value: fmt.Sprintf("Nudity (Explicit): %.0f%%\nNudity (Suggestive): %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%", a.Scores.NudityExplicit*100, a.Scores.NuditySuggestive*100, a.Scores.Offensive*100, a.Scores.AIGenerated*100), Inline: false},
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
					Value:  "Analyses an image URL for inappropriate content.\nArguments:\n- `image_url` (required): The Image URL to analyse\n- `advanced` (optional): `true` shows detailed category and subcategory scores for nudity, offensive content and AI usage",
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

	// Open session before registering commands so s.State.User is populated.
	if err := sess.Open(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bot is now online!")

	// Register the /analyse command (guild‑scoped if GUILD_ID is set, otherwise global).
	appID := sess.State.User.ID
	guildID := os.Getenv("GUILD_ID")
	cmd, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "analyse",
		Description: "Analyses an image URL for inappropriate content",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "image_url",
				Description: "The image URL to analyse",
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
		log.Fatalf("cannot create command: %v", err)
	}
	defer func() {
		if err := sess.ApplicationCommandDelete(appID, guildID, cmd.ID); err != nil {
			log.Println("failed to delete command:", err)
		}
	}()

	// Register /ping
	cmdPing, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Pong!",
	})
	if err != nil {
		log.Fatalf("cannot create command ping: %v", err)
	}
	defer func() {
		if err := sess.ApplicationCommandDelete(appID, guildID, cmdPing.ID); err != nil {
			log.Println("failed to delete command:", err)
		}
	}()

	// Register /help
	cmdHelp, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows a list of commands",
	})
	if err != nil {
		log.Fatalf("cannot create command help: %v", err)
	}
	defer func() {
		if err := sess.ApplicationCommandDelete(appID, guildID, cmdHelp.ID); err != nil {
			log.Println("failed to delete command:", err)
		}
	}()

	// Register /thresholds
	cmdThresholds, err := sess.ApplicationCommandCreate(appID, guildID, &discordgo.ApplicationCommand{
		Name:        "thresholds",
		Description: "Shows the current detection thresholds",
	})
	if err != nil {
		log.Fatalf("cannot create command thresholds: %v", err)
	}
	defer func() {
		if err := sess.ApplicationCommandDelete(appID, guildID, cmdThresholds.ID); err != nil {
			log.Println("failed to delete command:", err)
		}
	}()

	// Wait for exit signals.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Graceful shutdown for HTTP server.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if httpSrv != nil {
		_ = httpSrv.Shutdown(ctx)
	}
}
