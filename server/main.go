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

var (
	awestruck_env     = os.Getenv("AWESTRUCK_ENV")
	awestruck_api_key = os.Getenv("AWESTRUCK_API_KEY")
	openai_api_key    = os.Getenv("OPENAI_API_KEY")
	turn_server_host  = os.Getenv("TURN_SERVER_HOST")
	turn_username     = os.Getenv("TURN_USERNAME")
	turn_password     = os.Getenv("TURN_PASSWORD")
)

func main() {
	if awestruck_env == "" {
		awestruck_env = "development" // Default to development if not set
	}
	config.Init(
		awestruck_env,
		awestruck_api_key,
		openai_api_key,
		turn_server_host,
		turn_username,
		turn_password,
	)
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
