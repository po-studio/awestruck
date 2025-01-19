package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/po/awestruck/turn"
)

func main() {
	// why we need command line flags:
	// - allows runtime configuration
	// - supports different environments
	// - enables easy testing
	port := flag.Int("port", 3478, "UDP port for TURN/STUN")
	healthPort := flag.Int("health-port", 3479, "TCP port for health checks")
	flag.Parse()

	// why we need environment-based realm:
	// - matches domain in production (awestruck.io)
	// - uses localhost for development
	// - enables proper turn authentication
	realm := os.Getenv("TURN_REALM")
	if realm == "" {
		if os.Getenv("AWESTRUCK_ENV") == "production" {
			realm = "awestruck.io"
		} else {
			realm = "localhost"
		}
	}

	// why we need to log external ip:
	// - helps debug NAT traversal issues
	// - verifies elastic ip is set correctly
	// - confirms environment variable passing
	externalIP := os.Getenv("EXTERNAL_IP")
	log.Printf("[TURN] External IP: %q", externalIP)
	log.Printf("[TURN] Environment: %q", os.Getenv("AWESTRUCK_ENV"))

	server, err := turn.NewTurnServer(*port, realm)
	if err != nil {
		log.Fatalf("Failed to create TURN server: %v", err)
	}

	// why we need a separate health endpoint:
	// - provides consistent health checks across environments
	// - works with both docker and ecs
	// - uses tcp for reliable checking
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
		if err := http.ListenAndServe(fmt.Sprintf(":%d", *healthPort), nil); err != nil {
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
