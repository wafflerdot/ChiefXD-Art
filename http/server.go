package httputil

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
)

var server *http.Server

// StartServer starts a minimal HTTP server for health checks
func StartServer() {
	// Resolve port (default 8080 for Cloud Run)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Router with simple health endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Server instance
	server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Run server in background to avoid blocking the bot
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
}

// Shutdown gracefully shuts down the HTTP server
func Shutdown(ctx context.Context) error {
	if server != nil {
		return server.Shutdown(ctx)
	}
	return nil
}
