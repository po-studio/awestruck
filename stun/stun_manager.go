package stun

import (
	"fmt"
	"log"
	"sync"
)

// why we need a stun manager:
// - provides centralized control over stun server lifecycle
// - ensures clean startup and shutdown
// - allows for future expansion of stun functionality
type StunManager struct {
	server  *StunServer
	udpPort int
	tcpPort int
	mu      sync.Mutex
}

var (
	instance *StunManager
	once     sync.Once
)

func GetStunManager() *StunManager {
	once.Do(func() {
		instance = &StunManager{
			udpPort: 3478, // Default STUN port
			tcpPort: 3479, // Default health check port
		}
	})
	return instance
}

// why we need separate port setters:
// - allows independent configuration of stun and health ports
// - maintains backward compatibility
// - provides clear configuration interface
func (m *StunManager) SetUDPPort(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.udpPort = port
}

func (m *StunManager) SetTCPPort(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tcpPort = port
}

func (m *StunManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		return fmt.Errorf("STUN server already running")
	}

	server, err := NewStunServer(m.udpPort, m.tcpPort)
	if err != nil {
		return fmt.Errorf("failed to create STUN server: %v", err)
	}

	m.server = server
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("[STUN] Server error: %v", err)
		}
	}()

	log.Printf("[STUN] Manager started server on UDP port %d and TCP port %d", m.udpPort, m.tcpPort)
	return nil
}

func (m *StunManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		m.server.Stop()
	}
	return nil
}

func (m *StunManager) GetServerAddress() string {
	return fmt.Sprintf("stun:0.0.0.0:%d", m.udpPort)
}
