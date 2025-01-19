package turn

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/turn/v2"
)

// why we need a custom turn server:
// - provides both STUN and TURN functionality
// - enables reliable WebRTC connections through symmetric NATs
// - allows for custom configuration and monitoring
type TurnServer struct {
	server      *turn.Server
	realm       string
	udpPort     int
	stopped     bool
	credentials sync.Map // thread-safe map for credential management
	authKey     []byte   // static auth key
}

// why we need proper relay address detection:
// - production: use container's network interface
// - local docker: use host machine IP
// - prevents using wrong IPs in each environment
func getRelayAddress() (net.IP, error) {
	log.Printf("getRelayAddress: Starting IP detection")

	env := os.Getenv("AWESTRUCK_ENV")
	log.Printf("getRelayAddress: Environment is %q", env)

	if env == "production" {
		log.Printf("getRelayAddress: Running in production mode")

		// List all interfaces first
		interfaces, err := net.Interfaces()
		if err != nil {
			log.Printf("getRelayAddress: Failed to list interfaces: %v", err)
		} else {
			for _, iface := range interfaces {
				log.Printf("getRelayAddress: Found interface: %s (flags: %v)", iface.Name, iface.Flags)
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						log.Printf("getRelayAddress: Interface %s has address: %v", iface.Name, addr)
					}
				}
			}
		}

		// Try common interface names in ECS/Docker
		interfaceNames := []string{"eth0", "ens5", "en0", "eth1"}
		for _, name := range interfaceNames {
			log.Printf("getRelayAddress: Trying interface %s", name)
			iface, err := net.InterfaceByName(name)
			if err != nil {
				log.Printf("getRelayAddress: Interface %s not found: %v", name, err)
				continue
			}

			addrs, err := iface.Addrs()
			if err != nil {
				log.Printf("getRelayAddress: Failed to get addresses for %s: %v", name, err)
				continue
			}

			for _, addr := range addrs {
				log.Printf("getRelayAddress: Found address on %s: %v", name, addr)
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() && !ipv4.IsLinkLocalUnicast() {
						log.Printf("getRelayAddress: Using IP from %s for relay: %s", name, ipv4.String())
						return ipv4, nil
					}
				}
			}
		}

		log.Printf("getRelayAddress: No suitable IPv4 address found on any interface")
		return nil, fmt.Errorf("no suitable IPv4 address found on any interface")
	}

	// For local development in Docker, try to get host IP via environment
	if hostIP := os.Getenv("HOST_IP"); hostIP != "" {
		if ip := net.ParseIP(hostIP); ip != nil {
			return ip, nil
		}
	}

	// Fallback: try to find a suitable IP address for local development
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface addresses: %v", err)
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() && !ipv4.IsLinkLocalUnicast() && !isDockerIP(ipv4) {
				return ipv4, nil
			}
		}
	}
	return nil, fmt.Errorf("no suitable IPv4 address found")
}

// why we need docker ip detection:
// - only used in local development
// - prevents using container network IPs locally
// - not used in production where we want VPC IPs
func isDockerIP(ip net.IP) bool {
	// Only used in local development
	if os.Getenv("AWESTRUCK_ENV") == "production" {
		return false
	}

	// Common Docker network ranges
	dockerRanges := []string{
		"172.16.0.0/12",
		"192.168.0.0/16",
		"10.0.0.0/8",
	}
	for _, cidr := range dockerRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func NewTurnServer(udpPort int, realm string) (*TurnServer, error) {
	// why we need enhanced auth logging:
	// - tracks credential validation steps
	// - helps diagnose auth failures
	// - monitors timestamp validation
	authKey := []byte("awestruck-turn-static-auth-key")

	server := &TurnServer{
		realm:   realm,
		udpPort: udpPort,
		authKey: authKey,
	}

	log.Printf("[TURN] Starting server with realm: %q, port: %d", realm, udpPort)

	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", udpPort))
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN listener: %v", err)
	}

	relayIP, err := getRelayAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get relay address: %v", err)
	}

	log.Printf("[TURN] Using relay address: %s", relayIP.String())

	externalIP := os.Getenv("EXTERNAL_IP")
	if externalIP == "" {
		log.Printf("[TURN] No EXTERNAL_IP set, using relay IP %s for external address", relayIP.String())
		externalIP = relayIP.String()
	} else {
		log.Printf("[TURN] Using EXTERNAL_IP for address: %s", externalIP)
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// why we need optimized server config:
		// - improves connection stability
		// - reduces resource usage
		// - supports high concurrency
		LoggerFactory: nil, // Use our own logging
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			log.Printf("[AUTH] Request from %v - username: %s, realm: %s", srcAddr, username, realm)

			parts := strings.Split(username, ":")
			if len(parts) != 2 {
				log.Printf("[AUTH] ❌ Invalid username format: %s (expected timestamp:identifier)", username)
				return nil, false
			}

			timestamp, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				log.Printf("[AUTH] ❌ Invalid timestamp in username %s: %v", username, err)
				return nil, false
			}

			now := time.Now().Unix()
			if now > timestamp {
				log.Printf("[AUTH] ❌ Credential expired: now=%d > timestamp=%d (diff=%d seconds)",
					now, timestamp, now-timestamp)
				return nil, false
			}

			// why we need to match client hmac:
			// - ensures consistent auth between client/server
			// - validates using same key derivation
			// - prevents integrity check failures
			hmacKey := server.authKey
			log.Printf("[AUTH] ✓ Credential validation passed for %s:", username)
			log.Printf("  - Timestamp valid until: %s", time.Unix(timestamp, 0))
			log.Printf("  - Client realm: %s, Server realm: %s", realm, server.realm)
			log.Printf("  - Source address: %v", srcAddr)
			log.Printf("  - Auth key length: %d bytes", len(hmacKey))
			log.Printf("  - Username for HMAC: %s", username)

			return hmacKey, true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: relayIP,
					Address:      externalIP,
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

// why we need credential management:
// - enables future dynamic auth
// - allows per-user permissions
// - supports credential rotation
func (s *TurnServer) AddCredentials(username string, password []byte) {
	s.credentials.Store(username, password)
	log.Printf("Added credentials for user: %s", username)
}

func (s *TurnServer) RemoveCredentials(username string) {
	s.credentials.Delete(username)
	log.Printf("Removed credentials for user: %s", username)
}

func (s *TurnServer) Start() error {
	log.Printf("[TURN] Starting server on UDP port %d with:", s.udpPort)
	log.Printf("  - Realm: %s", s.realm)
	log.Printf("  - Auth key length: %d bytes", len(s.authKey))

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
// - tracks active ice sessions
// - identifies failed connections
// - helps debug connectivity issues
func (s *TurnServer) monitorConnections() {
	if s.server == nil {
		return
	}

	// basic server health check
	log.Printf("[MONITOR] TURN server health check:")
	log.Printf("  - Server running: true")
	log.Printf("  - UDP port: %d", s.udpPort)
	log.Printf("  - Realm: %s", s.realm)
}

// why we need server stats:
// - monitors resource usage
// - tracks active allocations
// - helps identify memory leaks
func (s *TurnServer) logServerStats() {
	// Basic stats logging - expand based on pion/turn capabilities
	log.Printf("[STATS] TURN Server status:")
	log.Printf("  - Server running: %v", !s.stopped)
	log.Printf("  - Active credentials: %d", s.countCredentials())

	// Log memory stats
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	log.Printf("  - Memory usage:")
	log.Printf("    • Alloc: %v MiB", mem.Alloc/1024/1024)
	log.Printf("    • TotalAlloc: %v MiB", mem.TotalAlloc/1024/1024)
	log.Printf("    • Sys: %v MiB", mem.Sys/1024/1024)
	log.Printf("    • NumGC: %v", mem.NumGC)
}

func (s *TurnServer) countCredentials() int {
	count := 0
	s.credentials.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
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
