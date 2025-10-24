package bot

import (
	"fmt"
	"strings"

	"github.com/alfredosa/GoDiscordBot/config"
	"github.com/bwmarrin/discordgo"
)

var BotId string
var goBot *discordgo.Session

func Start() {
	var err error
	cfg := config.ReadConfig()
	if err != nil {
		fmt.Println("Failed to read configuration:", err)
		return
	}

	goBot, err = discordgo.New("Bot " + cfg.Token)
	if err != nil {
		fmt.Println("Failed whilst initialising Discord session:", err)
		return
	}

	u, err := goBot.User("@me")
	if err != nil {
		fmt.Println("Failed to get current user:", err)
		return
	}

	BotId = u.ID

	goBot.AddHandler(messageHandler)

	err = goBot.Open()
	if err != nil {
		fmt.Println("Failed whilst opening connection to Discord:", err)
		return
	}

	fmt.Println("Bot connected!")
}

func messageHandler(s *discordgo.Session, e *discordgo.MessageCreate) {
	if e.Author.ID == BotId {
		return
	}

	prefix := config.BotPrefix
	if strings.HasPrefix(e.Content, prefix) {
		args := strings.Fields(e.Content)[strings.Index(e.Content, prefix):]
		cmd := args[0][len(prefix):]
		arguments := args[1:]

		switch cmd {
		case "ping":
			_, err := s.ChannelMessageSend(e.ChannelID, "Pong!")
			if err != nil {
				fmt.Println("Failed ping response:", err)
			}
		default:
			_, err := s.ChannelMessageSend(e.ChannelID, fmt.Sprintf("Unknown command: %q.", cmd))
			if err != nil {
				fmt.Println("Failed unknown command response:", err)
			}
		}
	}
}
