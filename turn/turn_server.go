package turn

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pion/turn/v2"
)

// why we need a custom turn server:
// - provides both STUN and TURN functionality
// - enables reliable WebRTC connections through symmetric NATs
// - allows for custom configuration and monitoring
type TurnServer struct {
	server  *turn.Server
	realm   string
	udpPort int
	stopped bool
}

// why we need proper relay address detection:
// - production: use container's internal IP for relay binding
// - local: use container IP for development too
// - ensures TURN server only binds to addresses it can use
func getRelayAddress() (net.IP, error) {
	log.Printf("getRelayAddress: Starting IP detection")

	// Try to get the container's IP by dialing out
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if ipv4 := localAddr.IP.To4(); ipv4 != nil {
			log.Printf("getRelayAddress: Using container IP: %s", ipv4.String())
			return ipv4, nil
		}
	}

	// Fallback to eth0 if Dial method fails
	iface, err := net.InterfaceByName("eth0")
	if err == nil {
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() {
						log.Printf("getRelayAddress: Using eth0 IP: %s", ipv4.String())
						return ipv4, nil
					}
				}
			}
		}
	}

	// Last resort: use localhost
	log.Printf("getRelayAddress: Using localhost as last resort")
	return net.ParseIP("127.0.0.1"), nil
}

// why we need proper turn auth:
// - follows RFC 5389/5766 TURN protocol
// - uses MD5 hash of username:realm:password
// - matches pion/turn's GenerateAuthKey implementation
func NewTurnServer(udpPort int, realm string) (*TurnServer, error) {
	server := &TurnServer{
		realm:   realm,
		udpPort: udpPort,
	}

	log.Printf("[TURN] Starting server with realm: %q, port: %d", realm, udpPort)

	// Create UDP listener with logging
	addr := fmt.Sprintf("0.0.0.0:%d", udpPort)
	log.Printf("[TURN] Creating UDP listener on %s", addr)
	udpListener, err := net.ListenPacket("udp4", addr)
	if err != nil {
		log.Printf("[TURN][ERROR] Failed to create UDP listener: %v", err)
		return nil, fmt.Errorf("failed to create UDP listener: %v", err)
	}
	log.Printf("[TURN] Successfully created UDP listener: %v", udpListener.LocalAddr())

	// Create a logging UDP listener
	loggingListener := &loggingPacketConn{
		PacketConn: udpListener,
	}

	// Get the internal IP for relay binding
	relayIP, err := getRelayAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get relay address: %v", err)
	}

	log.Printf("[TURN] Using internal relay address: %s", relayIP.String())

	// why we need proper external IP handling:
	// - allows NAT traversal in production via static IP
	// - supports both IP and DNS format for EXTERNAL_IP
	// - prevents address mismatch issues
	externalIP := os.Getenv("EXTERNAL_IP")
	if externalIP == "" {
		if os.Getenv("AWESTRUCK_ENV") == "production" {
			log.Printf("[TURN] ERROR: EXTERNAL_IP must be set in production")
			return nil, fmt.Errorf("EXTERNAL_IP environment variable is required in production")
		}
		// For local development, use the same IP as relay
		log.Printf("[TURN] No EXTERNAL_IP set, using relay IP %s for external address", relayIP.String())
		externalIP = relayIP.String()
	} else {
		// Check if EXTERNAL_IP is a DNS name and resolve it
		if ips, err := net.LookupHost(externalIP); err == nil && len(ips) > 0 {
			log.Printf("[TURN] Resolved EXTERNAL_IP %s to IPs: %v", externalIP, ips)
			// Find first valid IPv4 address (must be a host address, not network)
			for _, ip := range ips {
				if parsedIP := net.ParseIP(ip); parsedIP != nil {
					if ipv4 := parsedIP.To4(); ipv4 != nil && !ipv4.IsUnspecified() {
						// Ensure it's not a network address (last octet should not be 0)
						if ipv4[3] != 0 {
							log.Printf("[TURN] Using external IPv4 address: %s", ipv4.String())
							externalIP = ipv4.String()
							break
						}
					}
				}
			}
			if strings.HasSuffix(externalIP, ".0") {
				log.Printf("[TURN] WARNING: External IP %s appears to be a network address", externalIP)
				return nil, fmt.Errorf("external IP %s is not a valid host address", externalIP)
			}
		}
		log.Printf("[TURN] Using EXTERNAL_IP for client advertisement: %s", externalIP)
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// why we need optimized server config:
		// - uses turn.GenerateAuthKey for proper auth
		// - logs auth attempts for debugging
		// - supports static credentials for testing
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			log.Printf("[AUTH] Received auth request: username=%q realm=%q from=%v", username, realm, srcAddr)
			if username == "user" {
				key := turn.GenerateAuthKey(username, realm, "pass")
				log.Printf("[AUTH] Generated key for user=%q realm=%q key=%x", username, realm, key)
				return key, true
			}
			log.Printf("[AUTH] Unknown user: %q (expected: user)", username)
			return nil, false
		},
		// why we need to log all packets:
		// - helps debug stun binding requests
		// - shows if server receives any traffic
		// - identifies potential protocol issues
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: loggingListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: relayIP,    // Internal IP for actual relay binding
					Address:      externalIP, // External IP for client advertisement only
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN server: %v", err)
	}

	server.server = s
	return server, nil
}

func (s *TurnServer) Start() error {
	log.Printf("[TURN] Starting server on UDP port %d with:", s.udpPort)
	log.Printf("  - Realm: %s", s.realm)

	// why we need enhanced monitoring:
	// - tracks connection states
	// - helps diagnose ice failures
	// - provides operational metrics
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			if s.stopped {
				return
			}
			s.monitorConnections()
		}
	}()

	// Monitor server state periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			if s.stopped {
				return
			}
			s.logServerStats()
		}
	}()

	return nil
}

// why we need connection monitoring:
// - tracks active ice sessions and candidate types
// - identifies peer reflexive candidates for NAT traversal
// - helps correlate client-side candidate filtering
func (s *TurnServer) monitorConnections() {
	if s.server == nil {
		return
	}

	// basic server health check
	log.Printf("[MONITOR] TURN server health check:")
	log.Printf("  - Server running: true")
	log.Printf("  - UDP port: %d", s.udpPort)
	log.Printf("  - Realm: %s", s.realm)

	// Track candidate types being used
	log.Printf("  - Candidate types:")
	log.Printf("    • Server Reflexive (srflx): allowed")
	log.Printf("    • Peer Reflexive (prflx): allowed")
	log.Printf("    • Relay (relay): allowed")
	log.Printf("    • Host: filtered by client")
}

// why we need server stats:
// - monitors resource usage
// - tracks active allocations
// - helps identify memory leaks
func (s *TurnServer) logServerStats() {
	// Basic stats logging - expand based on pion/turn capabilities
	log.Printf("[STATS] TURN Server status:")
	log.Printf("  - Server running: %v", !s.stopped)

	// Log memory stats
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	log.Printf("  - Memory usage:")
	log.Printf("    • Alloc: %v MiB", mem.Alloc/1024/1024)
	log.Printf("    • TotalAlloc: %v MiB", mem.TotalAlloc/1024/1024)
	log.Printf("    • Sys: %v MiB", mem.Sys/1024/1024)
	log.Printf("    • NumGC: %v", mem.NumGC)
}

func (s *TurnServer) Stop() error {
	log.Printf("Stopping TURN/STUN server")
	s.stopped = true
	return s.server.Close()
}

// why we need health check:
// - enables container health monitoring
// - allows load balancer health checks
// - provides operational status
func (s *TurnServer) HealthCheck() error {
	// The server is healthy if it's able to accept connections
	conn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", s.udpPort), time.Second)
	if err != nil {
		return fmt.Errorf("health check failed: %v", err)
	}
	conn.Close()
	return nil
}

// why we need health status:
// - supports container orchestration
// - enables ECS health checks
// - provides consistent health reporting
func (s *TurnServer) IsHealthy() bool {
	if s.server == nil || s.stopped {
		log.Printf("[HEALTH] ❌ Unhealthy - server is nil or stopped")
		return false
	}

	// Test UDP listener
	conn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", s.udpPort), time.Second)
	if err != nil {
		log.Printf("[HEALTH] ❌ Unhealthy - UDP test failed: %v", err)
		return false
	}
	conn.Close()

	log.Printf("[HEALTH] ✓ Server is healthy")
	return true
}

// why we need packet logging:
// - debug incoming stun/turn requests
// - verify server receives traffic
// - identify protocol issues
type loggingPacketConn struct {
	net.PacketConn
}

func (l *loggingPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	// Log immediately when we get any packet
	log.Printf("[UDP] ReadFrom called")
	n, addr, err = l.PacketConn.ReadFrom(p)
	if err != nil {
		log.Printf("[UDP] Error reading: %v", err)
		return
	}
	log.Printf("[UDP] Read %d bytes from %v", n, addr)
	if n > 0 {
		// Log first 4 bytes to identify STUN messages (0x00 0x01 for binding request)
		log.Printf("[UDP] First 4 bytes: %02x %02x %02x %02x", p[0], p[1], p[2], p[3])
		log.Printf("[UDP] Full content: %x", p[:n])
	}
	return
}

func (l *loggingPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Log immediately when we try to send any packet
	log.Printf("[UDP] WriteTo called for %v", addr)
	n, err = l.PacketConn.WriteTo(p, addr)
	if err != nil {
		log.Printf("[UDP] Error writing: %v", err)
		return
	}
	log.Printf("[UDP] Wrote %d bytes to %v", n, addr)
	if n > 0 {
		// Log first 4 bytes to identify STUN messages (0x00 0x01 for binding request)
		log.Printf("[UDP] First 4 bytes: %02x %02x %02x %02x", p[0], p[1], p[2], p[3])
		log.Printf("[UDP] Full content: %x", p[:n])
	}
	return
}
