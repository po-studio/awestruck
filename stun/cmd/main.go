package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/po-studio/stun"
)

// why we need a standalone stun server:
// - better resource isolation
// - independent scaling
// - simpler monitoring and maintenance
func main() {
	// why we need environment-based configuration:
	// - allows runtime configuration
	// - supports container orchestration
	// - enables different settings per environment
	udpPort := 3478 // Default STUN port

	// TCP port for health checks is always UDP port + 1
	tcpPort := 3479

	// Create and start STUN server
	manager := stun.GetStunManager()
	manager.SetUDPPort(udpPort)
	manager.SetTCPPort(tcpPort)
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
