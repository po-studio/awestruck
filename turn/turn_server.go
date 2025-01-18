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
// - production: use task ENI IP for ECS/NLB
// - local docker: use host machine IP
// - prevents using internal container IPs
func getRelayAddress() (net.IP, error) {
	if os.Getenv("AWESTRUCK_ENV") == "production" {
		// In ECS, the task ENI IP is the first non-loopback IP on eth0
		iface, err := net.InterfaceByName("eth0")
		if err != nil {
			return nil, fmt.Errorf("failed to get eth0 interface: %v", err)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("failed to get eth0 addresses: %v", err)
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() {
					return ipv4, nil
				}
			}
		}
		return nil, fmt.Errorf("no IPv4 address found on eth0")
	}

	// For local development in Docker, try to get host IP via environment
	if hostIP := os.Getenv("HOST_IP"); hostIP != "" {
		if ip := net.ParseIP(hostIP); ip != nil {
			return ip, nil
		}
	}

	// Fallback: try to find a suitable IP address
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface addresses: %v", err)
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil && !ipv4.IsLoopback() && !isDockerIP(ipv4) {
				return ipv4, nil
			}
		}
	}
	return nil, fmt.Errorf("no suitable IPv4 address found")
}

// why we need docker ip detection:
// - prevents using container network IPs
// - ensures we use host network IPs
// - improves local development reliability
func isDockerIP(ip net.IP) bool {
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
			if credential, ok := server.credentials.Load(username); ok {
				return credential.([]byte), true
			}
			// Fallback to static key for default user
			if username == "default" {
				return authKey, true
			}
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
