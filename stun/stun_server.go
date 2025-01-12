package stun

import (
	"fmt"
	"log"
	"net"

	"github.com/pion/stun/v2"
)

// why we need a custom stun server:
// - reduces dependency on external services
// - provides better control over NAT traversal
// - allows for custom configuration and monitoring
type StunServer struct {
	listener *net.UDPConn
	port     int
}

func NewStunServer(port int) (*StunServer, error) {
	addr := &net.UDPAddr{
		Port: port,
		IP:   net.IPv4zero,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create STUN listener: %v", err)
	}

	return &StunServer{
		listener: conn,
		port:     port,
	}, nil
}

func (s *StunServer) Start() error {
	log.Printf("[STUN] Starting STUN server on port %d", s.port)

	buffer := make([]byte, 1024)

	for {
		n, remoteAddr, err := s.listener.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("[STUN] Error reading from UDP: %v", err)
			continue
		}

		go s.handleStunRequest(buffer[:n], remoteAddr)
	}
}

func (s *StunServer) handleStunRequest(data []byte, remoteAddr *net.UDPAddr) {
	message := &stun.Message{
		Raw: make([]byte, len(data)),
	}
	copy(message.Raw, data)

	if !stun.IsMessage(message.Raw) {
		return // Not a STUN message
	}

	if err := message.Decode(); err != nil {
		log.Printf("[STUN] Failed to decode message: %v", err)
		return
	}

	// Create response message
	response := &stun.Message{}
	response.TransactionID = message.TransactionID
	response.Type = stun.MessageType{Method: stun.MethodBinding, Class: stun.ClassSuccessResponse}

	// Add XOR-MAPPED-ADDRESS
	xorAddr := &stun.XORMappedAddress{
		IP:   remoteAddr.IP,
		Port: remoteAddr.Port,
	}
	if err := xorAddr.AddTo(response); err != nil {
		log.Printf("[STUN] Failed to add XOR-MAPPED-ADDRESS: %v", err)
		return
	}

	// Add SOFTWARE attribute
	software := stun.NewSoftware("pion-stun-server")
	if err := software.AddTo(response); err != nil {
		log.Printf("[STUN] Failed to add SOFTWARE: %v", err)
		return
	}

	// Add FINGERPRINT
	if err := stun.Fingerprint.AddTo(response); err != nil {
		log.Printf("[STUN] Failed to add FINGERPRINT: %v", err)
		return
	}

	// Send response
	if _, err := s.listener.WriteToUDP(response.Raw, remoteAddr); err != nil {
		log.Printf("[STUN] Failed to send response: %v", err)
		return
	}

	log.Printf("[STUN] Sent response to %s:%d", remoteAddr.IP, remoteAddr.Port)
}

func (s *StunServer) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
