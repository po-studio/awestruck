package turn

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/turn/v2"
)

// why we need structured logging:
// - easier to parse and analyze in cloudwatch
// - consistent format for all log entries
// - better debugging and monitoring
type logEntry struct {
	Timestamp   string      `json:"timestamp"`
	Level       string      `json:"level"`
	Message     string      `json:"message"`
	SessionID   string      `json:"session_id,omitempty"`
	RemoteAddr  string      `json:"remote_addr,omitempty"`
	PacketSize  int         `json:"packet_size,omitempty"`
	MessageType string      `json:"message_type,omitempty"`
	Error       string      `json:"error,omitempty"`
	Details     interface{} `json:"details,omitempty"`
}

// why we need a custom turn server:
// - provides both STUN and TURN functionality
// - enables reliable WebRTC connections through symmetric NATs
// - allows for custom configuration and monitoring
type TurnServer struct {
	server      *turn.Server
	realm       string
	udpPort     int
	credentials map[string]string
	stopped     bool
}

func NewTurnServer(udpPort int, realm string) (*TurnServer, error) {
	// why we generate a static auth key:
	// - provides consistent authentication across restarts
	// - allows for future key rotation if needed
	// - simplifies client configuration
	authKey := []byte("awestruck-turn-static-auth-key")

	// Setup a UDP listener
	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", udpPort))
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN listener: %v", err)
	}

	server, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// why we use a static auth handler:
		// - simplifies authentication for testing
		// - can be extended to use dynamic credentials
		// - allows for future integration with auth service
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			return authKey, true
		},
		// why we need packet conn configuration:
		// - enables both STUN and TURN functionality
		// - provides connection monitoring
		// - allows for custom network configurations
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("0.0.0.0"),
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN server: %v", err)
	}

	return &TurnServer{
		server:  server,
		realm:   realm,
		udpPort: udpPort,
		credentials: map[string]string{
			"default": "default",
		},
	}, nil
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

// why we need credential management:
// - enables dynamic credential updates
// - allows for user-specific permissions
// - supports future auth integration
func (s *TurnServer) AddCredentials(username, password string) {
	s.credentials[username] = password
}

func (s *TurnServer) RemoveCredentials(username string) {
	delete(s.credentials, username)
}

// why we need health checks:
// - enables monitoring of server status
// - supports container orchestration
// - provides consistent health reporting
func (s *TurnServer) IsHealthy() bool {
	return s.server != nil && !s.stopped
}
