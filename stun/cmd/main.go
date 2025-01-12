package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/po-studio/stun"
)

// why we need a standalone stun server:
// - better resource isolation
// - independent scaling
// - simpler monitoring and maintenance
func main() {
	// Get port from environment or use default
	portStr := os.Getenv("STUN_PORT")
	port := 3478 // Default STUN port
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	// Create and start STUN server
	manager := stun.GetStunManager()
	manager.SetPort(port)
	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start STUN server: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down STUN server...")

	if err := manager.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
