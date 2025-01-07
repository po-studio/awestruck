package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/po-studio/go-webrtc-server/config"
	"github.com/po-studio/go-webrtc-server/routes"
)

var (
	awestruck_env  = os.Getenv("AWESTRUCK_ENV")
	openai_api_key = os.Getenv("OPENAI_API_KEY")
)

func main() {
	if awestruck_env == "" {
		awestruck_env = "development" // Default to development if not set
	}
	config.Init(awestruck_env, openai_api_key)
	log.Printf("Starting server in %s environment", awestruck_env)

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
