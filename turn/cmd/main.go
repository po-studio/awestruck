package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/po/awestruck/turn"
)

func main() {
	// Load and validate configuration
	cfg, err := turn.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create TURN server with validated config
	server, err := turn.NewTurnServer(turn.ServerConfig{
		UDPPort:     cfg.Port,
		Realm:       cfg.Realm,
		Environment: cfg.Environment,
		ExternalIP:  cfg.ExternalIP,
		Credentials: cfg.Credentials,
	})
	if err != nil {
		log.Fatalf("Failed to create TURN server: %v", err)
	}

	// Start health check server
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			if server.IsHealthy() {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Unhealthy"))
			}
		})
		if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.HealthPort), nil); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start TURN server: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	server.Stop()
}
