package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN must be set in environment variables")
	}

	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()

	// Command handler: /analyse <image_url>
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Only respond to application command interactions.
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "analyse" {
			return
		}

		// Extract the `image_url` option.
		var imageURL string
		for _, opt := range i.ApplicationCommandData().Options {
			if opt.Name == "image_url" {
				imageURL = opt.StringValue()
				break
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

		// Run analysis.
		a, err := AnalyseImageURL(imageURL)
		if err != nil {
			msg := fmt.Sprintf("Analysis failed: %v", err)
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}

		// Build human‑readable fields.
		//reasons := "none"
		//if len(a.Reasons) > 0 {
		//	reasons = strings.Join(a.Reasons, ", ")
		//}

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
			//{Name: "Reasons", Value: reasons, Inline: false},
			{Name: "Results", Value: fmt.Sprintf("Nudity: %.0f%%\nOffensive: %.0f%%\nAI Generated: %.0f%%", a.Scores.Nudity*100, a.Scores.Offensive*100, a.Scores.AIGenerated*100), Inline: false},
			{Name: "Text Flags", Value: textCounts, Inline: false},
		}
		//if a.MediaURI != "" {
		//	fields = append(fields, &discordgo.MessageEmbedField{
		//		Name:   "Media URI",
		//		Value:  a.MediaURI,
		//		Inline: false,
		//	})
		//}

		embed := &discordgo.MessageEmbed{
			Title:       "Image Analysis",
			Description: fmt.Sprintf("Analysis results for: %s", imageURL),
			Color:       0x00BFA5,
			Fields:      fields,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Bot created by wafflerdot",
			},
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
		Description: "Analyse an image URL. Checks for nudity, offensive content, TOS text, and AI usage.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "image_url",
				Description: "The image URL to analyse",
				Required:    true,
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

	// Wait for exit signals.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
