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
	server *StunServer
	port   int
	mu     sync.Mutex
}

var (
	instance *StunManager
	once     sync.Once
)

func GetStunManager() *StunManager {
	once.Do(func() {
		instance = &StunManager{
			port: 3478, // Default STUN port
		}
	})
	return instance
}

func (m *StunManager) SetPort(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.port = port
}

func (m *StunManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		return fmt.Errorf("STUN server already running")
	}

	server, err := NewStunServer(m.port)
	if err != nil {
		return fmt.Errorf("failed to create STUN server: %v", err)
	}

	m.server = server
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("[STUN] Server error: %v", err)
		}
	}()

	log.Printf("[STUN] Manager started server on port %d", m.port)
	return nil
}

func (m *StunManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		if err := m.server.Stop(); err != nil {
			return fmt.Errorf("failed to stop STUN server: %v", err)
		}
		m.server = nil
	}
	return nil
}

func (m *StunManager) GetServerAddress() string {
	return fmt.Sprintf("stun:0.0.0.0:%d", m.port)
}
