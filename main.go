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
	// Load environment variables from .env
	_ = godotenv.Load()

	// ----------------------------------------
	// Permissions persistence (DB or JSON fallback)
	// ----------------------------------------
	dsn := os.Getenv("PERMS_DSN")
	dialect := os.Getenv("PERMS_DIALECT") // postgres | mysql
	if dsn != "" {
		if dialect == "" {
			dialect = "postgres"
		}
		if err := perms.ConfigureDB(dialect, dsn); err != nil {
			log.Fatalf("permissions DB config failed: %v", err)
		}
		log.Printf("permissions: DB configured (dialect=%s)", dialect)
	} else {
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
	}

	// Initialise thresholds store and load values from DB if present
	if err := thresholdsStore.Init(perms); err != nil {
		log.Println("thresholds init error:", err)
	}

	// ----------------------------------------
	// Start lightweight HTTP health server
	// ----------------------------------------
	startHTTPServer()

	// ----------------------------------------
	// Discord session setup
	// ----------------------------------------
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

	// Register gateway and command handlers
	registerHandlers(sess)

	// Open the WebSocket connection to Discord before creating commands
	if err := sess.Open(); err != nil {
		log.Fatal(err)
	}
	log.Println("Bot is now online!")

	// Create slash commands (global or guild scoped depending on GUILD_ID)
	registerCommands(sess)

	// ----------------------------------------
	// Block until termination, then graceful shutdown
	// ----------------------------------------
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(ctx)
	}
}
