package main

import (
	"errors"
	"log"
	"net/http"
	"os"
)

var httpServer *http.Server

// startHTTPServer starts a minimal HTTP server for Cloud Run health/readiness.
func startHTTPServer() {
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
	httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Run server in background to avoid blocking the bot
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
}
