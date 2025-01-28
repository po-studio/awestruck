package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/po-studio/server/config"
	"github.com/po-studio/server/routes"
)

func main() {
	// Initialize config from environment variables
	if err := config.Init(config.LoadFromEnv()); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	cfg := config.Get()
	log.Printf("Starting server in %s environment", cfg.Environment)

	// Graceful shutdown
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")
		os.Exit(0)
	}()

	router := routes.NewRouter()

	fmt.Println("Server started at http://0.0.0.0:8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", router))
}
