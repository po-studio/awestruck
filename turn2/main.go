// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a standard TURN server following RFC 5766
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

// why we need verbose logging:
// - helps diagnose connection issues
// - tracks allocation and permission lifecycle
// - provides visibility into turn operations
type verboseLogger struct {
	prefix string
}

func (v *verboseLogger) Trace(msg string) {
	log.Printf("[TRACE][%s] %s", v.prefix, msg)
}

func (v *verboseLogger) Tracef(format string, args ...interface{}) {
	log.Printf("[TRACE][%s] %s", v.prefix, fmt.Sprintf(format, args...))
}

func (v *verboseLogger) Debug(msg string) {
	log.Printf("[DEBUG][%s] %s", v.prefix, msg)
}

func (v *verboseLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG][%s] %s", v.prefix, fmt.Sprintf(format, args...))
}

func (v *verboseLogger) Info(msg string) {
	log.Printf("[INFO][%s] %s", v.prefix, msg)
}

func (v *verboseLogger) Infof(format string, args ...interface{}) {
	log.Printf("[INFO][%s] %s", v.prefix, fmt.Sprintf(format, args...))
}

func (v *verboseLogger) Warn(msg string) {
	log.Printf("[WARN][%s] %s", v.prefix, msg)
}

func (v *verboseLogger) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN][%s] %s", v.prefix, fmt.Sprintf(format, args...))
}

func (v *verboseLogger) Error(msg string) {
	log.Printf("[ERROR][%s] %s", v.prefix, msg)
}

func (v *verboseLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR][%s] %s", v.prefix, fmt.Sprintf(format, args...))
}

type verboseLoggerFactory struct{}

func (f *verboseLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	return &verboseLogger{prefix: scope}
}

func main() {
	publicIP := flag.String("public-ip", "", "IP Address that TURN can be contacted by.")
	port := flag.Int("port", 3478, "Listening port.")
	users := flag.String("users", "", "List of username and password (e.g. \"user=pass,user=pass\")")
	realm := flag.String("realm", "localhost", "Realm (defaults to \"localhost\")")
	flag.Parse()

	if len(*publicIP) == 0 {
		log.Fatalf("'public-ip' is required")
	} else if len(*users) == 0 {
		log.Fatalf("'users' is required")
	}

	log.Printf("[TURN] Starting server on %s:%d", *publicIP, *port)

	// Create a logger
	loggerFactory := &verboseLoggerFactory{}
	logger := loggerFactory.NewLogger("turn")

	// Parse users
	usersMap := map[string][]byte{}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(*users, -1) {
		usersMap[kv[1]] = turn.GenerateAuthKey(kv[1], *realm, kv[2])
		logger.Debugf("Added user: %s", kv[1])
	}

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		log.Panicf("[TURN] Failed to create UDP listener: %s", err)
	}
	logger.Debugf("UDP listener created on 0.0.0.0:%d (external: %s)", *port, *publicIP)

	// Configure TURN server
	config := turn.ServerConfig{
		Realm: *realm,
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if key, ok := usersMap[username]; ok {
				logger.Debugf("Auth success - username: %s, client: %s", username, srcAddr)
				return key, true
			}
			logger.Debugf("Auth failed - username: %s, client: %s", username, srcAddr)
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(*publicIP),
					Address:      "0.0.0.0",
				},
			},
		},
		LoggerFactory: loggerFactory,
	}

	// Create and start the server
	server, err := turn.NewServer(config)
	if err != nil {
		log.Fatalf("[TURN] Failed to create server: %v", err)
	}

	// Wait for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	if err = server.Close(); err != nil {
		log.Panicf("[TURN] Failed to close server: %s", err)
	}
}
