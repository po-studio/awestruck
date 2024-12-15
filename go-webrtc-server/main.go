package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/po-studio/go-webrtc-server/routes"
)

var (
	environment = os.Getenv("ENVIRONMENT")
)

func main() {
	if environment == "" {
		environment = "development" // Default to development if not set
	}
	log.Printf("Starting server in %s environment", environment)

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
