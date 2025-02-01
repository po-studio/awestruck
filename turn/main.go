// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a standard TURN server following RFC 5766
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

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
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	loggerFactory := &verboseLoggerFactory{}
	turnLogger := loggerFactory.NewLogger("turn")

	turnRealm := os.Getenv("TURN_REALM")
	if turnRealm == "" {
		panic("TURN_REALM is not set")
	}

	fixedUsername := os.Getenv("TURN_USERNAME")
	if fixedUsername == "" {
		panic("TURN_USERNAME is not set")
	}

	fixedPassword := os.Getenv("TURN_PASSWORD")
	if fixedPassword == "" {
		panic("TURN_PASSWORD is not set")
	}

	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		panic("PUBLIC_IP is not set")
	}

	minPortStr := os.Getenv("TURN_MIN_PORT")
	if minPortStr == "" {
		panic("TURN_MIN_PORT is not set")
	}

	maxPortStr := os.Getenv("TURN_MAX_PORT")
	if maxPortStr == "" {
		panic("TURN_MAX_PORT is not set")
	}

	minPort, err := strconv.Atoi(minPortStr)
	if err != nil {
		panic("TURN_MIN_PORT is not a valid integer")
	}

	maxPort, err := strconv.Atoi(maxPortStr)
	if err != nil {
		panic("TURN_MAX_PORT is not a valid integer")
	}

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
	if err != nil {
		logger.Fatalf("Failed to create UDP listener: %v", err)
	}
	defer udpListener.Close()

	// Create TURN server configuration
	config := turn.ServerConfig{
		Realm: turnRealm,
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			turnLogger.Debugf("Auth request from %s for user %s", srcAddr.String(), username)
			if username == fixedUsername {
				turnLogger.Debugf("Auth success for user %s", username)
				return turn.GenerateAuthKey(username, realm, fixedPassword), true
			}
			turnLogger.Debugf("Auth failed for user %s", username)
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: net.ParseIP(publicIP),
					Address:      "0.0.0.0",
					MinPort:      uint16(minPort),
					MaxPort:      uint16(maxPort),
				},
				PermissionHandler: turn.DefaultPermissionHandler,
			},
		},
		LoggerFactory: loggerFactory,
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
	turnLogger.Infof("Using relay ports %d-%d", minPort, maxPort)

	// why we need a health check endpoint:
	// - enables aws load balancer health checks
	// - provides container readiness probe
	// - supports local development testing
	//
	// NB: ideally this would be UDP since this is what the turn server uses
	// for its core functionality, but ECS health checks require TCP
	healthPort := "3479"
	if portStr := os.Getenv("HEALTH_PORT"); portStr != "" {
		healthPort = portStr
	}

	go func() {
		healthListener, err := net.Listen("tcp", "0.0.0.0:"+healthPort)
		if err != nil {
			turnLogger.Errorf("Failed to create health check listener: %v", err)
			return
		}
		defer healthListener.Close()

		turnLogger.Info("Health check endpoint running on TCP " + healthListener.Addr().String())

		for {
			conn, err := healthListener.Accept()
			if err != nil {
				turnLogger.Errorf("Failed to accept health check connection: %v", err)
				continue
			}

			go func(c net.Conn) {
				defer c.Close()
				c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			}(conn)
		}
	}()

	// Block main goroutine forever
	select {}
}
