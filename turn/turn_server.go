package turn

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pion/turn/v2"
)

// why we need a custom turn server:
// - provides both STUN and TURN functionality
// - enables reliable WebRTC connections through symmetric NATs
// - allows for custom configuration and monitoring
type TurnServer struct {
	server        *turn.Server
	realm         string
	signalingPort int
	stopped       bool
	ctx           context.Context
	cancel        context.CancelFunc
	environment   string
	config        *Config
	dtlsStats     *dtlsStats
}

// why we need credential management:
// - enables secure authentication
// - supports dynamic credentials
// - prevents hardcoded passwords
type Credentials struct {
	Username string
	Password string
}

// why we need server configuration:
// - centralizes all server settings
// - validates required fields
// - provides type safety
type ServerConfig struct {
	SignalingPort int
	Realm         string
	Environment   string
	ExternalIP    string
	Credentials   Credentials
}

// why we need external address management:
// - handles nlb ip changes through dns
// - provides thread-safe access to ips
// - ensures reliable client connectivity
type externalAddressManager struct {
	dnsName string
	ips     []net.IP
	// why we need a stable primary ip:
	// - prevents ice candidate mismatches during connection setup
	// - maintains consistent external address for webrtc peers
	// - avoids connection drops from ip changes during dns updates
	primaryIP net.IP
	mu        sync.RWMutex
}

// why we need dns resolution retries:
// - handles temporary dns failures
// - prevents unnecessary server restarts
// - ensures high availability
func (m *externalAddressManager) resolveIPsWithRetry() error {
	maxRetries := 3
	backoff := time.Second

	for i := 0; i < maxRetries; i++ {
		if err := m.resolveIPs(); err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to resolve IPs after %d attempts: %v", maxRetries, err)
			}
			log.Printf("[TURN][WARN] DNS resolution attempt %d failed: %v, retrying in %v", i+1, err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to resolve IPs after %d attempts", maxRetries)
}

func (m *externalAddressManager) resolveIPs() error {
	ips, err := net.LookupHost(m.dnsName)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %v", m.dnsName, err)
	}

	validIPs := make([]net.IP, 0)
	for _, ip := range ips {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			if ipv4 := parsedIP.To4(); ipv4 != nil {
				// why we need public ip validation:
				// - ensures external ip is routable
				// - prevents using private addresses in production
				// - maintains nat traversal capability
				if isPublicIP(ipv4) {
					validIPs = append(validIPs, ipv4)
				} else {
					log.Printf("[TURN][WARN] Skipping private IP: %v", ipv4)
				}
			}
		}
	}

	if len(validIPs) == 0 {
		return fmt.Errorf("no valid public IPv4 addresses found for %s", m.dnsName)
	}

	m.mu.Lock()
	m.ips = validIPs
	// why we need primary ip persistence:
	// - first resolved ip becomes the stable external address
	// - subsequent dns updates won't change the primary ip
	// - ensures ice candidates remain valid throughout session
	if m.primaryIP == nil {
		m.primaryIP = validIPs[0]
		log.Printf("[TURN] Set primary external IP: %v", m.primaryIP)
	} else {
		log.Printf("[TURN] Keeping existing primary IP: %v (newly resolved IPs: %v)", m.primaryIP, validIPs)
	}
	m.mu.Unlock()

	return nil
}

// why we need public ip validation:
// - ensures external ip is routable
// - prevents using private addresses in production
// - maintains nat traversal capability
func isPublicIP(ip net.IP) bool {
	// In development mode, allow private IPs
	if os.Getenv("AWESTRUCK_ENV") == "development" {
		return true
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}

	// Check against private IP ranges
	privateBlocks := []string{
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 Link-Local
		"127.0.0.0/8",    // RFC1122 Loopback
	}

	for _, block := range privateBlocks {
		_, subnet, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if subnet.Contains(ip) {
			return false
		}
	}
	return true
}

// why we need custom relay address generation:
// - ensures turn server only binds to container ip
// - advertises nlb ip to clients
// - handles aws network load balancer setup
type customRelayAddressGenerator struct {
	relayIP     net.IP
	addrManager *externalAddressManager
	server      *TurnServer
}

func (g *customRelayAddressGenerator) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	logWithTime("[TURN] Allocating relay on %s", g.relayIP.String())
	conn, err := net.ListenPacket(network, fmt.Sprintf("%s:0", g.relayIP.String())) // Use port 0 for dynamic allocation
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to listen: %v", err)
		return nil, nil, err
	}

	// why we need stable ip allocation:
	// - ensures all ice candidates use same external ip
	// - prevents connection failures from ip inconsistency
	// - maintains stable relay endpoints for webrtc peers
	g.addrManager.mu.RLock()
	if g.addrManager.primaryIP == nil {
		g.addrManager.mu.RUnlock()
		conn.Close()
		logWithTime("[TURN][ERROR] No primary external IP available")
		return nil, nil, fmt.Errorf("no primary external IP available")
	}
	externalIP := g.addrManager.primaryIP
	g.addrManager.mu.RUnlock()

	// Create enhanced connection with logging
	enhancedConn := &enhancedPacketConn{
		PacketConn: conn,
		server:     g.server,
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	logWithTime("[TURN] Allocated relay: local=%v:%d external=%v:%d", g.relayIP, localAddr.Port, externalIP, localAddr.Port)
	return enhancedConn, &net.UDPAddr{
		IP:   externalIP,
		Port: localAddr.Port,
	}, nil
}

func (g *customRelayAddressGenerator) AllocateConn(network string, requestedPort int) (net.Conn, net.Addr, error) {
	// TCP connections not supported for TURN relay
	return nil, nil, fmt.Errorf("TCP allocation not supported")
}

func (g *customRelayAddressGenerator) Validate() error {
	if g.relayIP == nil {
		return fmt.Errorf("relay IP is nil")
	}
	if g.addrManager == nil {
		return fmt.Errorf("address manager is nil")
	}
	return nil
}

// why we need container ip detection:
// - ensures proper binding in fargate
// - handles different network setups
// - provides reliable local addressing
func getContainerIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, fmt.Errorf("failed to detect container IP: %v", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	if ipv4 := localAddr.IP.To4(); ipv4 != nil {
		log.Printf("[TURN] Using container IP: %s", ipv4.String())
		return ipv4, nil
	}

	return nil, fmt.Errorf("no valid IPv4 address found")
}

// why we need periodic dns resolution:
// - handles nlb ip changes
// - maintains up-to-date external addresses
// - ensures continuous service
func (m *externalAddressManager) startPeriodicResolution(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second) // More frequent checks for NLB
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.resolveIPsWithRetry(); err != nil {
					log.Printf("[TURN][ERROR] Periodic DNS resolution failed: %v", err)
				}
			}
		}
	}()
}

// why we need dtls metrics:
// - tracks handshake success rate
// - monitors connection health
// - helps identify connection patterns
type dtlsStats struct {
	sync.RWMutex
	handshakePackets int
	activeSessions   int
}

func newDTLSStats() *dtlsStats {
	return &dtlsStats{}
}

func NewTurnServer(config *Config) (*TurnServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	server := &TurnServer{
		realm:         config.Realm,
		signalingPort: config.SignalingPort,
		ctx:           ctx,
		cancel:        cancel,
		environment:   config.Environment,
		config:        config,
		dtlsStats:     newDTLSStats(),
	}

	log.Printf("[TURN] Starting server with realm: %q, port: %d", config.Realm, config.SignalingPort)
	log.Printf("[TURN] Environment: %s", config.Environment)
	log.Printf("[TURN] Auth user: %s", config.Credentials.Username)

	// Create UDP listener with logging
	addr := fmt.Sprintf("0.0.0.0:%d", config.SignalingPort)
	log.Printf("[TURN] Creating UDP listener on %s", addr)
	udpListener, err := net.ListenPacket("udp4", addr)
	if err != nil {
		log.Printf("[TURN][ERROR] Failed to create UDP listener: %v", err)
		return nil, fmt.Errorf("failed to create UDP listener: %v", err)
	}
	log.Printf("[TURN] Successfully created UDP listener: %v", udpListener.LocalAddr())

	// Create a logging UDP listener
	loggingListener := &enhancedPacketConn{
		PacketConn: udpListener,
	}

	// Get the container IP for relay binding
	relayIP, err := getContainerIP()
	if err != nil {
		return nil, fmt.Errorf("failed to get container IP: %v", err)
	}
	log.Printf("[TURN] Using container IP for relay binding: %v", relayIP)

	// Initialize external IP manager
	externalAddr := config.ExternalIP
	if externalAddr == "" {
		if config.Environment == "production" {
			return nil, fmt.Errorf("EXTERNAL_IP must be set in production")
		}
		// For development, use the container's IP as the external address
		externalAddr = relayIP.String()
		log.Printf("[TURN] Development mode: using container IP %s as external address", externalAddr)
	}

	externalIPManager := &externalAddressManager{
		dnsName: externalAddr,
	}

	// Always verify and resolve external IP with retries
	if err := externalIPManager.resolveIPsWithRetry(); err != nil {
		if config.Environment == "production" {
			return nil, fmt.Errorf("failed to resolve external IPs: %v", err)
		}
		// For development with IP address, initialize directly
		if ip := net.ParseIP(externalAddr); ip != nil {
			if !isPublicIP(ip) {
				log.Printf("[TURN][WARN] Using private IP in development mode: %s", ip)
			}
			externalIPManager.mu.Lock()
			externalIPManager.ips = []net.IP{ip}
			externalIPManager.mu.Unlock()
			log.Printf("[TURN] Development mode: using direct IP %s", ip)
		} else {
			return nil, fmt.Errorf("invalid IP address in development mode: %s", externalAddr)
		}
	}

	// Always start periodic DNS resolution to handle NLB changes
	externalIPManager.startPeriodicResolution(ctx)

	// Store credentials for auth handler
	username := config.Credentials.Username
	password := config.Credentials.Password
	log.Printf("[TURN] Using credentials for user: %s", username)

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: config.Realm,
		AuthHandler: func(u string, realm string, srcAddr net.Addr) ([]byte, bool) {
			log.Printf("[AUTH] Received auth request from %v for user: %s", srcAddr, u)
			if u != username {
				log.Printf("[AUTH] Unknown user: %s (expected: %s)", u, username)
				return nil, false
			}
			key := turn.GenerateAuthKey(u, realm, password)
			log.Printf("[AUTH] Generated key for user=%q realm=%q", u, realm)
			return key, true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: loggingListener,
				RelayAddressGenerator: &customRelayAddressGenerator{
					relayIP:     relayIP,
					addrManager: externalIPManager,
					server:      server,
				},
			},
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create TURN server: %v", err)
	}

	server.server = s
	return server, nil
}

func (s *TurnServer) Start() error {
	log.Printf("[TURN] Starting server on UDP port %d with:", s.signalingPort)
	log.Printf("  - Realm: %s", s.realm)

	// why we need periodic monitoring:
	// - tracks server health and performance
	// - logs stats at reasonable intervals
	// - prevents log spam while maintaining visibility
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.monitorConnections()
				s.logServerStats()
			}
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

	log.Printf("[MONITOR] TURN server health check:")
	log.Printf("  - Server running: true")
	log.Printf("  - UDP port: %d", s.signalingPort)
	log.Printf("  - Realm: %s", s.realm)

	log.Printf("  - Candidate types:")
	log.Printf("    • Server Reflexive (srflx): allowed")
	log.Printf("    • Peer Reflexive (prflx): allowed")
	log.Printf("    • Relay (relay): allowed")
	log.Printf("    • Host: filtered by client")

	// why we need dtls monitoring:
	// - tracks handshake progress
	// - identifies connection failures early
	// - helps debug ice connectivity issues
	log.Printf("  - DTLS Status (last minute):")
	log.Printf("    • Handshake packets received: %d", s.getDTLSStats())
	log.Printf("    • Active DTLS sessions: %d", s.getActiveSessions())
}

// why we need server stats:
// - monitors resource usage
// - tracks active allocations
// - helps identify memory leaks
func (s *TurnServer) logServerStats() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	log.Printf("[STATS] TURN Server status:")
	log.Printf("  - Server running: %v", !s.stopped)
	log.Printf("  - Memory usage:")
	log.Printf("    • Alloc: %v MiB", mem.Alloc/1024/1024)
	log.Printf("    • TotalAlloc: %v MiB", mem.TotalAlloc/1024/1024)
	log.Printf("    • Sys: %v MiB", mem.Sys/1024/1024)
	log.Printf("    • NumGC: %v", mem.NumGC)
}

func (s *TurnServer) Stop() error {
	log.Printf("Stopping TURN/STUN server")
	s.stopped = true
	s.cancel()
	return s.server.Close()
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
	log.Printf("[HEALTH] ✓ Server is healthy")
	return true
}

// why we need focused logging:
// - tracks critical turn server events
// - includes timestamps for correlation
// - maintains consistent log format
func logWithTime(format string, v ...interface{}) {
	log.Printf("[%s] %s", time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), fmt.Sprintf(format, v...))
}

// why we need dtls packet tracking:
// - counts successful handshakes
// - helps identify connection issues
// - provides metrics for monitoring
func (s *TurnServer) incrementDTLSHandshakePackets() {
	s.dtlsStats.Lock()
	s.dtlsStats.handshakePackets++
	s.dtlsStats.Unlock()
}

func (s *TurnServer) incrementActiveSessions() {
	s.dtlsStats.Lock()
	s.dtlsStats.activeSessions++
	s.dtlsStats.Unlock()
}

func (s *TurnServer) decrementActiveSessions() {
	s.dtlsStats.Lock()
	if s.dtlsStats.activeSessions > 0 {
		s.dtlsStats.activeSessions--
	}
	s.dtlsStats.Unlock()
}

func (s *TurnServer) getDTLSStats() int {
	s.dtlsStats.RLock()
	defer s.dtlsStats.RUnlock()
	return s.dtlsStats.handshakePackets
}

func (s *TurnServer) getActiveSessions() int {
	s.dtlsStats.RLock()
	defer s.dtlsStats.RUnlock()
	return s.dtlsStats.activeSessions
}

// why we need enhanced packet connection:
// - combines logging and cleanup
// - tracks dtls and stun traffic
type enhancedPacketConn struct {
	net.PacketConn
	server *TurnServer
}

func (e *enhancedPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = e.PacketConn.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}

	// Check for DTLS handshake packets (first byte 20-63)
	if n > 0 && p[0] >= 20 && p[0] <= 63 {
		logWithTime("[TURN][DTLS] Received handshake packet type %d from %s", p[0], addr.String())
		if e.server != nil {
			e.server.incrementDTLSHandshakePackets()
			// Record new session on ClientHello (type 22)
			if p[0] == 22 {
				e.server.incrementActiveSessions()
			}
		}
	}

	// Log STUN messages too
	if n >= 20 { // Minimum STUN message size
		messageType := uint16(p[0])<<8 | uint16(p[1])
		if messageType&0xC000 == 0 { // STUN messages
			logWithTime("[STUN] Received message type: 0x%04x from %v", messageType, addr)
		}
	}

	return n, addr, err
}

func (e *enhancedPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Check for DTLS handshake packets before writing
	if len(p) > 0 && p[0] >= 20 && p[0] <= 63 {
		logWithTime("[TURN][DTLS] Sending handshake packet type %d to %s", p[0], addr.String())
	}

	// Log STUN messages too
	if len(p) >= 20 { // Minimum STUN message size
		messageType := uint16(p[0])<<8 | uint16(p[1])
		if messageType&0xC000 == 0 { // STUN message
			logWithTime("[STUN] Sent message type: 0x%04x to %v", messageType, addr)
		}
	}

	return e.PacketConn.WriteTo(p, addr)
}

func (e *enhancedPacketConn) Close() error {
	return e.PacketConn.Close()
}
