// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a standard TURN server following RFC 5766
package main

import (
	"fmt"
	"log"
	"net"
	"os"

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

// why we need proper turn configuration:
// - ensures correct relay address allocation
// - maps client addresses properly
// - enables reliable media relay
func main() {
	// Create a logger that writes to stdout
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	// Create verbose logger for TURN operations
	loggerFactory := &verboseLoggerFactory{}
	turnLogger := loggerFactory.NewLogger("turn")

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
	if err != nil {
		logger.Fatalf("Failed to create UDP listener: %v", err)
	}
	defer udpListener.Close()

	// Create TURN server configuration
	config := turn.ServerConfig{
		Realm: "awestruck.audio",
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			turnLogger.Debugf("Auth request from %s for user %s", srcAddr.String(), username)
			// Use fixed credentials for testing
			if username == "awestruck_user" {
				turnLogger.Debugf("Auth success for user %s", username)
				return turn.GenerateAuthKey(username, realm, "verySecurePassword1234567890abcdefghijklmnop"), true
			}
			turnLogger.Debugf("Auth failed for user %s", username)
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("192.168.4.82"), // Your server's public IP
					Address:      "0.0.0.0",
				},
			},
		},
		LoggerFactory: loggerFactory, // Use our verbose logger
	}

	// Create server instance
	server, err := turn.NewServer(config)
	if err != nil {
		logger.Fatalf("Failed to create TURN server: %v", err)
	}
	defer func() {
		if err = server.Close(); err != nil {
			logger.Printf("Failed to close TURN server: %v", err)
		}
	}()

	turnLogger.Info("TURN server is running on UDP " + udpListener.LocalAddr().String())

	// Block main goroutine forever
	select {}
}
