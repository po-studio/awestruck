package turn

import (
	"fmt"
	"log"
	"net"
	"os"
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
	// why we use a static auth key initially:
	// - provides consistent authentication across restarts
	// - simplifies client configuration
	// - matches client-side configuration
	authKey := []byte("awestruck-turn-static-auth-key")

	server := &TurnServer{
		realm:   realm,
		udpPort: udpPort,
	}

	// why we need to bind to all interfaces:
	// - allows connections from any network interface
	// - works for both local and cloud environments
	// - supports container networking
	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", udpPort))
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN listener: %v", err)
	}

	relayIP, err := getRelayAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get relay address: %v", err)
	}

	log.Printf("TURN server using relay address: %s", relayIP.String())

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// why we need dynamic auth handling:
		// - enables future per-user credentials
		// - allows credential rotation
		// - supports auth service integration
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			log.Printf("Auth request from %v - username: %s, realm: %s", srcAddr, username, realm)
			if credential, ok := server.credentials.Load(username); ok {
				log.Printf("Found stored credentials for user %s", username)
				return credential.([]byte), true
			}
			// Fallback to static key for default user
			if username == "default" {
				log.Printf("Using default credentials for user %s", username)
				return authKey, true
			}
			log.Printf("Auth failed - no credentials found for user %s", username)
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: relayIP,
					Address:      relayIP.String(),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN server: %v", err)
	}

	server.server = s
	// Set default credential
	server.credentials.Store("default", authKey)

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
	log.Printf("Starting TURN/STUN server on UDP port %d", s.udpPort)
	return nil
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
	return s.server != nil && !s.stopped
}
