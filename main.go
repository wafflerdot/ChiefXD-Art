package main

import (
	"context"
	"log"
	"os"
	"os/signal"
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

	// Configure and load permissions persistence
	permsFile := os.Getenv("PERMS_FILE")
	if permsFile == "" {
		permsFile = "permissions.json"
	}
	perms.ConfigureFile(permsFile)
	if err := perms.LoadFromFile(); err != nil {
		log.Println("failed to load permissions file:", err)
	} else {
		log.Println("permissions loaded from:", permsFile)
	}

	// Start HTTP health server for Cloud Run
	startHTTPServer()

	// Discord Bot
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN must be set in environment variables")
	}

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

	// Wire handlers (READY + slash commands)
	registerHandlers(sess)

	// Open session before registering commands so s.State.User is populated
	if err := sess.Open(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bot is now online!")

	// Create slash commands (global or guild scoped depending on GUILD_ID)
	registerCommands(sess)

	// Wait for exit signals.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Graceful shutdown for HTTP server.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(ctx)
	}
}
