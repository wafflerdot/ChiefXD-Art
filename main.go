package main

import (
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

func main() {
	_ = godotenv.Load()

	// Stupid HTTP stuff
	http.HandleFunc("/", handler)

	// Determine port for HTTP service
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("defaulting to port %s", port)
	}

	// Start HTTP server
	log.Printf("listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}

	// Discord Bot
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN must be set in environment variables")
	}

	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()

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
					fmt.Fprintf(&b, "%s: %.0f%%\n", k, m[k]*100)
				}
				val := strings.TrimRight(b.String(), "\n")
				return &discordgo.MessageEmbedField{Name: title, Value: val, Inline: false}
			}

			// Sorted text counts for deterministic output.
			textCounts := "none"
			if len(aa.TextCounts) > 0 {
				keys := make([]string, 0, len(aa.TextCounts))
				for k := range aa.TextCounts {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var b strings.Builder
				for _, k := range keys {
					fmt.Fprintf(&b, "%s: %d\n", k, aa.TextCounts[k])
				}
				textCounts = strings.TrimRight(b.String(), "\n")
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
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   "Text Flags",
				Value:  textCounts,
				Inline: false,
			})

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

		// Standard analysis (unchanged output).
		a, err := AnalyseImageURL(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}

		// Sorted text counts for deterministic output.
		textCounts := "none"
		if len(a.TextCounts) > 0 {
			keys := make([]string, 0, len(a.TextCounts))
			for k := range a.TextCounts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var b strings.Builder
			for _, k := range keys {
				fmt.Fprintf(&b, "%s: %d\n", k, a.TextCounts[k])
			}
			textCounts = strings.TrimRight(b.String(), "\n")
		}

		fields := []*discordgo.MessageEmbedField{
			{Name: "Safe Image", Value: fmt.Sprintf("%t", a.Allowed), Inline: true},
			{Name: "Results", Value: fmt.Sprintf("Nudity: %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%", a.Scores.Nudity*100, a.Scores.Offensive*100, a.Scores.AIGenerated*100), Inline: false},
			{Name: "Text Flags", Value: textCounts, Inline: false},
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
					Value:  "Analyses an image URL for inappropriate content.\nArguments:\n- `image_url` (required): The Image URL to analyse\n- `advanced` (optional): `true` shows detailed category and subcategory scores for nudity, offensive content, AI usage, and bad text.",
					Inline: false,
				},
				{
					Name:   "/ping",
					Value:  "Displays the bot's response time.",
					Inline: false,
				},
				{
					Name:   "/help",
					Value:  "Shows this message.",
					Inline: false,
				},
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

	// Wait for exit signals.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func handler(w http.ResponseWriter, r *http.Request) {
	name := os.Getenv("NAME")
	if name == "" {
		name = "World"
	}
	fmt.Fprintf(w, "Hello %s!\n", name)
}
