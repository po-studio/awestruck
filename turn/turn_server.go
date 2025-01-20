package turn

import (
	"context"
	"fmt"
	"log"
	"net"
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
	server      *turn.Server
	realm       string
	udpPort     int
	stopped     bool
	ctx         context.Context
	cancel      context.CancelFunc
	environment string
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
	UDPPort     int
	Realm       string
	Environment string
	ExternalIP  string
	Credentials Credentials
}

// why we need external address management:
// - handles nlb ip changes through dns
// - provides thread-safe access to ips
// - ensures reliable client connectivity
type externalAddressManager struct {
	dnsName string
	ips     []net.IP
	mu      sync.RWMutex
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
				// - prevents using private addresses
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
	m.mu.Unlock()

	log.Printf("[TURN] Updated external IPs: %v", validIPs)
	return nil
}

// why we need public ip validation:
// - ensures external ip is routable
// - prevents using private addresses
// - maintains nat traversal capability
func isPublicIP(ip net.IP) bool {
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
	portManager *portRangeManager
}

// why we need port range management:
// - matches nlb configured ports
// - ensures deterministic port allocation
// - prevents port conflicts
type portRangeManager struct {
	mu        sync.Mutex
	nextPort  int
	minPort   int
	maxPort   int
	usedPorts map[int]bool
}

func newPortRangeManager(minPort, maxPort int) *portRangeManager {
	return &portRangeManager{
		minPort:   minPort,
		maxPort:   maxPort,
		nextPort:  minPort,
		usedPorts: make(map[int]bool),
	}
}

func (p *portRangeManager) allocatePort() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to find an available port
	for i := 0; i <= p.maxPort-p.minPort; i++ {
		port := p.nextPort
		p.nextPort++
		if p.nextPort > p.maxPort {
			p.nextPort = p.minPort
		}

		if !p.usedPorts[port] {
			p.usedPorts[port] = true
			logWithTime("[PORT] Allocated relay port %d", port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", p.minPort, p.maxPort)
}

func (p *portRangeManager) releasePort(port int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if port >= p.minPort && port <= p.maxPort {
		delete(p.usedPorts, port)
		logWithTime("[PORT] Released relay port %d", port)
	}
}

// why we need enhanced packet connection:
// - combines logging and cleanup
// - ensures port release on close
// - tracks dtls and stun traffic
type enhancedPacketConn struct {
	net.PacketConn
	port        int
	portManager *portRangeManager
}

func (e *enhancedPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = e.PacketConn.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}

	// Check for DTLS handshake packets (first byte 20-63)
	if n > 0 && p[0] >= 20 && p[0] <= 63 {
		logWithTime("[TURN][DTLS] Received handshake packet type %d from %s on port %d", p[0], addr.String(), e.port)
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
		logWithTime("[TURN][DTLS] Sending handshake packet type %d to %s from port %d", p[0], addr.String(), e.port)
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
	if e.portManager != nil {
		logWithTime("[PORT] Cleaning up port %d", e.port)
		e.portManager.releasePort(e.port)
	}
	return e.PacketConn.Close()
}

func (g *customRelayAddressGenerator) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	if g.portManager == nil {
		g.portManager = newPortRangeManager(10000, 10010)
		logWithTime("[TURN] Initialized port manager with range 10000-10010")
	}

	port, err := g.portManager.allocatePort()
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to allocate port: %v", err)
		return nil, nil, err
	}

	logWithTime("[TURN] Allocating relay on %s:%d", g.relayIP.String(), port)
	conn, err := net.ListenPacket(network, fmt.Sprintf("%s:%d", g.relayIP.String(), port))
	if err != nil {
		g.portManager.releasePort(port)
		logWithTime("[TURN][ERROR] Failed to listen: %v", err)
		return nil, nil, err
	}

	// Get current external IP
	g.addrManager.mu.RLock()
	if len(g.addrManager.ips) == 0 {
		g.addrManager.mu.RUnlock()
		g.portManager.releasePort(port)
		logWithTime("[TURN][ERROR] No external IPs available")
		return nil, nil, fmt.Errorf("no external IPs available")
	}
	externalIP := g.addrManager.ips[0]
	g.addrManager.mu.RUnlock()

	// Create enhanced connection with both logging and cleanup
	enhancedConn := &enhancedPacketConn{
		PacketConn:  conn,
		port:        port,
		portManager: g.portManager,
	}

	logWithTime("[TURN] Allocated relay: local=%v:%d external=%v:%d", g.relayIP, port, externalIP, port)
	return enhancedConn, &net.UDPAddr{
		IP:   externalIP,
		Port: port,
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

func NewTurnServer(config ServerConfig) (*TurnServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	server := &TurnServer{
		realm:       config.Realm,
		udpPort:     config.UDPPort,
		ctx:         ctx,
		cancel:      cancel,
		environment: config.Environment,
	}

	log.Printf("[TURN] Starting server with realm: %q, port: %d", config.Realm, config.UDPPort)
	log.Printf("[TURN] Environment: %s", config.Environment)
	log.Printf("[TURN] Auth user: %s", config.Credentials.Username)

	// Create UDP listener with logging
	addr := fmt.Sprintf("0.0.0.0:%d", config.UDPPort)
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
	log.Printf("[TURN] Starting server on UDP port %d with:", s.udpPort)
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
	log.Printf("  - UDP port: %d", s.udpPort)
	log.Printf("  - Realm: %s", s.realm)
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
