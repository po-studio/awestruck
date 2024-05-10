package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/po-studio/go-webrtc-server/routes"
)

func main() {
	flag.Parse()

	// Graceful shutdown
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")
		os.Exit(0)
	}()

	router := routes.NewRouter()

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
