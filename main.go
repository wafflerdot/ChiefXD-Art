package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	httputil "ChiefXD-Art/http"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

const (
	FooterText    = "Bot created by wafflerdot"
	FooterIconURL = "https://cdn.discordapp.com/avatars/757284149793914900/a_ce62565c5aa6303ce7c932c83f227bf8.gif?size=1024&animated=true"
)

var globalRegister bool

func main() {
	globalRegister = false
	// Load environment variables from .env
	_ = godotenv.Load()

	// ----------------------------------------
	// Permissions persistence
	// ----------------------------------------
	dsn := os.Getenv("PERMS_DSN")
	dialect := os.Getenv("PERMS_DIALECT") // postgres | mysql
	if dsn == "" {
		log.Fatal("PERMS_DSN must be set (database connection required)")
	}
	if dialect == "" {
		dialect = "postgres"
	}
	if err := perms.ConfigureDB(dialect, dsn); err != nil {
		log.Fatalf("permissions DB config failed: %v", err)
	}
	log.Printf("permissions: DB configured (dialect=%s)", dialect)

	// Pre-warm caches for optimal first-request latency
	perms.WarmWhitelistCache()

	// Initialise thresholds store and load values from DB if present
	if err := thresholdsStore.Init(perms); err != nil {
		log.Println("thresholds init error:", err)
	}

	// ----------------------------------------
	// Start lightweight HTTP health server
	// ----------------------------------------
	httputil.StartServer()

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
	globalRegister = true

	// Mark startup as complete - onGuildCreate will now process new guild joins
	StartupComplete()

	// ----------------------------------------
	// Block until termination, then graceful shutdown
	// ----------------------------------------
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httputil.Shutdown(ctx)
}
