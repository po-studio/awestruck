package turn

import (
	"bytes"
	"context"
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

// remoteAddressTracker maintains mappings between source addresses and real client addresses,
// as well as local ports to remote addresses. This is critical for:
// - preserving client IP information through the NLB proxy protocol
// - ensuring ICE candidates have correct address information
// - maintaining proper address relationships for WebRTC connections
type remoteAddressTracker struct {
	clientAddrs   map[string]*net.UDPAddr // maps source addr -> real client addr
	localToRemote map[string]*net.UDPAddr // maps local port -> remote addr
	mu            sync.RWMutex
}

func newRemoteAddressTracker() *remoteAddressTracker {
	return &remoteAddressTracker{
		clientAddrs:   make(map[string]*net.UDPAddr),
		localToRemote: make(map[string]*net.UDPAddr),
	}
}

// why we need proxy protocol support:
// - preserves original client ip through nlb
// - enables proper nat traversal
// - maintains correct raddr in candidates
type proxyProtocolConn struct {
	net.PacketConn
	tracker *remoteAddressTracker
}

// proxyProtocolConn wraps a UDP connection to handle proxy protocol headers
// this enables proper client IP preservation when operating behind an NLB
// and ensures WebRTC connections maintain correct address information
func newProxyProtocolConn(conn net.PacketConn) *proxyProtocolConn {
	return &proxyProtocolConn{
		PacketConn: conn,
		tracker:    newRemoteAddressTracker(),
	}
}

// why we need proxy protocol parsing:
// - extracts real client ip from proxy headers
// - handles both v1 and v2 proxy protocol
// - maintains backward compatibility
func (p *proxyProtocolConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	n, addr, err = p.PacketConn.ReadFrom(b)
	if err != nil {
		return
	}

	// Skip DTLS packets (content type 20-23)
	if n > 0 && (b[0] >= 20 && b[0] <= 23) {
		return
	}

	// Only process if we got enough data for a PROXY header
	if n < 16 {
		return
	}

	// Check for PROXY protocol v2 signature
	if bytes.Equal(b[:12], []byte("PROXY TCP4 ")) {
		// Parse v1 header
		header := string(b[:n])
		parts := strings.Split(header, " ")
		if len(parts) >= 6 && parts[0] == "PROXY" {
			clientIP := net.ParseIP(parts[2])
			clientPort := parts[4]
			if clientIP != nil {
				p.tracker.mu.Lock()
				clientAddr := &net.UDPAddr{
					IP:   clientIP,
					Port: atoi(clientPort),
				}
				p.tracker.clientAddrs[addr.String()] = clientAddr
				if udpAddr, ok := addr.(*net.UDPAddr); ok {
					localKey := fmt.Sprintf("%d", udpAddr.Port)
					p.tracker.localToRemote[localKey] = clientAddr
				}
				p.tracker.mu.Unlock()
				logWithTime("[PROXY] Parsed client address: %v:%s", clientIP, clientPort)
			}
		}
	}

	return
}

func (p *proxyProtocolConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	return p.PacketConn.WriteTo(b, addr)
}

func (p *proxyProtocolConn) GetRealClientAddr(addr net.Addr) *net.UDPAddr {
	p.tracker.mu.RLock()
	defer p.tracker.mu.RUnlock()
	if realAddr, ok := p.tracker.clientAddrs[addr.String()]; ok {
		return realAddr
	}
	return nil
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
	conn, err := net.ListenPacket(network, fmt.Sprintf("%s:0", g.relayIP.String()))
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to listen: %v", err)
		return nil, nil, err
	}

	g.addrManager.mu.RLock()
	if g.addrManager.primaryIP == nil {
		g.addrManager.mu.RUnlock()
		conn.Close()
		logWithTime("[TURN][ERROR] No primary external IP available")
		return nil, nil, fmt.Errorf("no primary external IP available")
	}
	externalIP := g.addrManager.primaryIP
	g.addrManager.mu.RUnlock()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Create enhanced connection without proxy protocol for relay connections
	enhancedConn := newEnhancedPacketConn(conn, g.server)

	// Create relay address with external IP
	relayAddr := &net.UDPAddr{
		IP:   externalIP,
		Port: localAddr.Port,
	}

	logWithTime("[TURN] Allocated relay %v (external: %v)", localAddr, relayAddr)
	return enhancedConn, relayAddr, nil
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
	loggingListener := newEnhancedPacketConn(udpListener, server)

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
	log.Printf("[TURN] Using credentials for user: %s", username)

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: config.Realm,
		AuthHandler: func(u string, realm string, srcAddr net.Addr) ([]byte, bool) {
			log.Printf("[AUTH] Received auth request from %v for user: %s", srcAddr, u)
			if u != config.Credentials.Username {
				log.Printf("[AUTH] Unknown user: %s (expected: %s)", u, config.Credentials.Username)
				return nil, false
			}
			key := turn.GenerateAuthKey(u, realm, config.Credentials.Password)
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

	health := s.runHealthChecks()
	udpListener, turnAlloc, stunBinding, nlbReachable, lastCheck := health.getStatus()
	isHealthy := udpListener && turnAlloc && stunBinding && nlbReachable

	if isHealthy {
		logWithTime("[HEALTH] ✓ Server is healthy")
	} else {
		logWithTime("[HEALTH] ❌ Server is unhealthy:")
		logWithTime("  - UDP Listener: %v", udpListener)
		logWithTime("  - TURN Allocation: %v", turnAlloc)
		logWithTime("  - STUN Binding: %v", stunBinding)
		logWithTime("  - NLB Reachable: %v", nlbReachable)
		logWithTime("  - Last Check: %v", lastCheck)
	}

	return isHealthy
}

// why we need comprehensive health checks:
// - verifies all critical server components
// - ensures proper nlb integration
// - maintains service reliability
type healthCheck struct {
	mu           sync.RWMutex
	udpListener  bool
	turnAlloc    bool
	stunBinding  bool
	nlbReachable bool
	lastCheck    time.Time
}

func (h *healthCheck) setStatus(field *bool, value bool) {
	h.mu.Lock()
	*field = value
	h.mu.Unlock()
}

func (h *healthCheck) getStatus() (bool, bool, bool, bool, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.udpListener, h.turnAlloc, h.stunBinding, h.nlbReachable, h.lastCheck
}

func (s *TurnServer) runHealthChecks() *healthCheck {
	health := newHealthCheck()
	health.mu.Lock()
	health.lastCheck = time.Now()
	health.mu.Unlock()

	// Check UDP listener
	if conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", s.signalingPort+1)); err == nil {
		conn.Close()
		health.setStatus(&health.udpListener, true)
	}

	// Check STUN binding
	if err := s.checkSTUNBinding(); err == nil {
		health.setStatus(&health.stunBinding, true)
	}

	// Check TURN allocation
	if err := s.checkTURNAllocation(); err == nil {
		health.setStatus(&health.turnAlloc, true)
	}

	// Check NLB reachability
	if err := s.checkNLBReachability(); err == nil {
		health.setStatus(&health.nlbReachable, true)
	}

	return health
}

// why we need stun binding check:
// - verifies basic connectivity
// - ensures address discovery works
// - validates server responsiveness
func (s *TurnServer) checkSTUNBinding() error {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	// Send STUN binding request
	msg := []byte{0x00, 0x01, 0x00, 0x00, // STUN binding request
		0x21, 0x12, 0xa4, 0x42, // Magic cookie
		0x00, 0x00, 0x00, 0x00, // Transaction ID
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00}

	_, err = conn.WriteTo(msg, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: s.signalingPort,
	})
	if err != nil {
		return fmt.Errorf("failed to send STUN binding request: %v", err)
	}

	// Wait for response
	conn.SetReadDeadline(time.Now().Add(time.Second))
	resp := make([]byte, 1024)
	n, _, err := conn.ReadFrom(resp)
	if err != nil {
		return fmt.Errorf("failed to receive STUN binding response: %v", err)
	}

	// Verify STUN response
	if n < 20 || resp[0] != 0x01 || resp[1] != 0x01 {
		return fmt.Errorf("invalid STUN binding response")
	}

	return nil
}

// why we need turn allocation check:
// - verifies relay functionality
// - ensures proper authentication
// - validates allocation lifecycle
func (s *TurnServer) checkTURNAllocation() error {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	// Send TURN allocation request
	msg := createTURNAllocateRequest()

	_, err = conn.WriteTo(msg, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: s.signalingPort,
	})
	if err != nil {
		return fmt.Errorf("failed to send TURN allocation request: %v", err)
	}

	// Wait for response
	conn.SetReadDeadline(time.Now().Add(time.Second))
	resp := make([]byte, 1024)
	n, _, err := conn.ReadFrom(resp)
	if err != nil {
		return fmt.Errorf("failed to receive TURN allocation response: %v", err)
	}

	// Verify TURN response
	if n < 20 || resp[0] != 0x01 {
		return fmt.Errorf("invalid TURN allocation response")
	}

	return nil
}

// why we need nlb reachability check:
// - verifies aws infrastructure
// - ensures proper load balancing
// - validates external connectivity
func (s *TurnServer) checkNLBReachability() error {
	if s.config.Environment != "production" {
		return nil // Skip in development
	}

	// Resolve NLB hostname
	ips, err := net.LookupHost(s.config.ExternalIP)
	if err != nil {
		return fmt.Errorf("failed to resolve NLB hostname: %v", err)
	}

	// Try to connect to each IP
	for _, ip := range ips {
		conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", ip, s.signalingPort), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
	}

	return fmt.Errorf("failed to reach NLB")
}

// Helper function to create TURN allocation request
func createTURNAllocateRequest() []byte {
	return []byte{
		0x00, 0x03, // TURN Allocate request
		0x00, 0x00, // Message length
		0x21, 0x12, 0xa4, 0x42, // Magic cookie
		0x00, 0x00, 0x00, 0x00, // Transaction ID
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
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
// - provides connection monitoring
// - tracks traffic patterns
// - helps debug connectivity issues
type enhancedPacketConn struct {
	net.PacketConn
	server          *TurnServer
	onNewClientAddr func(*net.UDPAddr)
	// why we need session tracking:
	// - prevents duplicate session counting
	// - ensures accurate metrics
	// - helps debug connection issues
	activeSessions map[string]bool
	mu             sync.RWMutex
}

func newEnhancedPacketConn(conn net.PacketConn, server *TurnServer) *enhancedPacketConn {
	return &enhancedPacketConn{
		PacketConn:     conn,
		server:         server,
		activeSessions: make(map[string]bool),
	}
}

func (c *enhancedPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = c.PacketConn.ReadFrom(p)
	if err == nil && addr != nil {
		// Track DTLS handshake packets (they start with content type 22)
		if n > 0 && p[0] == 22 {
			logWithTime("[DTLS] Received handshake packet from %v", addr)
			c.server.incrementDTLSHandshakePackets()

			// Track new DTLS sessions
			sessionKey := addr.String()
			c.mu.Lock()
			if !c.activeSessions[sessionKey] {
				c.activeSessions[sessionKey] = true
				c.mu.Unlock()
				c.server.handleNewSession()
				logWithTime("[DTLS] Started new session for %v", addr)
			} else {
				c.mu.Unlock()
			}
		} else if n > 0 && p[0] >= 20 && p[0] <= 23 {
			logWithTime("[DTLS] Received packet type %d from %v", p[0], addr)
		}

		if proxyConn, ok := c.PacketConn.(*proxyProtocolConn); ok {
			if clientAddr := proxyConn.GetRealClientAddr(addr); clientAddr != nil && c.onNewClientAddr != nil {
				c.onNewClientAddr(clientAddr)
			}
		}
		logWithTime("[TURN] Received %d bytes from %v", n, addr)
	}
	return
}

func (c *enhancedPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	n, err = c.PacketConn.WriteTo(p, addr)
	if err == nil && addr != nil {
		logWithTime("[TURN] Sent %d bytes to %v", n, addr)
	}
	return
}

func (c *enhancedPacketConn) Close() error {
	// End all active sessions before closing
	c.mu.Lock()
	for sessionKey := range c.activeSessions {
		delete(c.activeSessions, sessionKey)
		c.server.handleSessionEnd()
	}
	c.mu.Unlock()
	return c.PacketConn.Close()
}

// Helper function to convert string to int
func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// why we need session tracking:
// - monitors active dtls sessions
// - helps identify connection issues
// - provides metrics for monitoring
func (s *TurnServer) handleNewSession() {
	s.incrementActiveSessions()
}

func (s *TurnServer) handleSessionEnd() {
	s.decrementActiveSessions()
}

func newHealthCheck() *healthCheck {
	return &healthCheck{}
}
