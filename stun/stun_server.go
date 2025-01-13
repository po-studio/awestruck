package stun

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/pion/stun/v2"
)

// why we need a custom stun server:
// - reduces dependency on external services
// - provides better control over NAT traversal
// - allows for custom configuration and monitoring
type StunServer struct {
	listener       *net.UDPConn
	port           int
	healthListener net.Listener
}

func NewStunServer(port int) (*StunServer, error) {
	logWithTime("INFO", "Creating STUN server on port %d", port)

	addr := &net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	logWithTime("INFO", "Attempting to bind to %s:%d", addr.IP.String(), addr.Port)

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		logWithTime("ERROR", "Failed to create STUN listener: %v", err)
		return nil, fmt.Errorf("failed to create STUN listener on %s:%d: %v", addr.IP.String(), port, err)
	}

	logWithTime("INFO", "Successfully bound to UDP address")

	// why we need a separate tcp health check:
	// - provides reliable container health monitoring
	// - allows load balancer health checks
	// - simpler than checking udp stun protocol
	healthAddr := fmt.Sprintf("0.0.0.0:%d", port)
	healthListener, err := net.Listen("tcp", healthAddr)
	if err != nil {
		logWithTime("ERROR", "Failed to create health check listener: %v", err)
		conn.Close()
		return nil, fmt.Errorf("failed to create health check listener on %s: %v", healthAddr, err)
	}

	logWithTime("INFO", "Successfully created TCP health check listener")

	return &StunServer{
		listener:       conn,
		port:           port,
		healthListener: healthListener,
	}, nil
}

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

func logWithTime(level string, format string, v ...interface{}) {
	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   fmt.Sprintf(format, v...),
	}

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal log entry: %v", err)
		return
	}

	log.Println(string(jsonBytes))
}

func logWithContext(level string, msg string, ctx map[string]interface{}) {
	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   msg,
	}

	for k, v := range ctx {
		switch k {
		case "session_id":
			entry.SessionID = v.(string)
		case "remote_addr":
			entry.RemoteAddr = v.(string)
		case "packet_size":
			entry.PacketSize = v.(int)
		case "message_type":
			entry.MessageType = v.(string)
		case "error":
			entry.Error = v.(string)
		default:
			if entry.Details == nil {
				entry.Details = make(map[string]interface{})
			}
			entry.Details.(map[string]interface{})[k] = v
		}
	}

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal log entry: %v", err)
		return
	}

	log.Println(string(jsonBytes))
}

func (s *StunServer) Start() error {
	logWithTime("INFO", "Starting STUN server on port %d", s.port)

	if s.listener == nil {
		return fmt.Errorf("STUN server not properly initialized: nil listener")
	}

	logWithTime("INFO", "UDP listener started successfully")

	// Start UDP STUN handler
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, remoteAddr, err := s.listener.ReadFromUDP(buffer)
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					logWithTime("INFO", "UDP listener closed")
					return
				}
				logWithTime("ERROR", "Failed to read UDP packet: %v", err)
				continue
			}

			logWithTime("DEBUG", "Received %d bytes from %s", n, remoteAddr.String())

			go s.handleStunRequest(s.listener, buffer[:n], remoteAddr)
		}
	}()

	// Start TCP health check handler
	go func() {
		logWithTime("INFO", "Starting TCP health check handler on port %d", s.port)
		for {
			conn, err := s.healthListener.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					logWithTime("INFO", "Health check listener closed")
					return
				}
				logWithTime("ERROR", "Failed to accept health check connection: %v", err)
				continue
			}
			remoteAddr := conn.RemoteAddr().String()
			logWithTime("INFO", "Received health check connection from %s", remoteAddr)

			// why we need to send a response:
			// - confirms server is truly operational
			// - allows health check to verify response
			// - helps with debugging
			_, err = conn.Write([]byte("healthy\n"))
			if err != nil {
				logWithTime("ERROR", "Failed to send health check response: %v", err)
			}

			conn.Close()
			logWithTime("INFO", "Successfully handled health check from %s", remoteAddr)
		}
	}()

	return nil
}

func (s *StunServer) handleStunRequest(conn *net.UDPConn, packet []byte, addr *net.UDPAddr) {
	ctx := map[string]interface{}{
		"remote_addr": addr.String(),
		"packet_size": len(packet),
	}

	logWithContext("DEBUG", "Received STUN packet", ctx)

	m := &stun.Message{
		Raw: packet,
	}

	if err := m.Decode(); err != nil {
		ctx["error"] = err.Error()
		logWithContext("ERROR", "Failed to decode STUN message", ctx)
		return
	}

	ctx["message_type"] = m.Type.String()
	ctx["transaction_id"] = fmt.Sprintf("%x", m.TransactionID)
	logWithContext("DEBUG", "Decoded STUN message", ctx)

	if m.Type == stun.BindingRequest {
		logWithContext("INFO", "Processing binding request", ctx)

		resp := &stun.Message{
			Type: stun.BindingSuccess,
		}

		xorAddr := &stun.XORMappedAddress{
			IP:   addr.IP,
			Port: addr.Port,
		}

		ctx["xor_mapped_address"] = fmt.Sprintf("%s:%d", addr.IP.String(), addr.Port)
		logWithContext("DEBUG", "Adding XOR-MAPPED-ADDRESS", ctx)

		if err := resp.Build(
			stun.BindingSuccess,
			stun.NewTransactionIDSetter(m.TransactionID),
			xorAddr,
			stun.NewSoftware("po-studio/stun-server"),
			stun.Fingerprint,
		); err != nil {
			ctx["error"] = err.Error()
			logWithContext("ERROR", "Failed to build STUN response", ctx)
			return
		}

		if _, err := conn.WriteToUDP(resp.Raw, addr); err != nil {
			ctx["error"] = err.Error()
			logWithContext("ERROR", "Failed to send STUN response", ctx)
			return
		}

		logWithContext("INFO", "Successfully sent binding response", ctx)
	} else {
		ctx["unsupported_type"] = m.Type.String()
		logWithContext("WARN", "Received unsupported STUN message type", ctx)
	}
}

func (s *StunServer) Stop() {
	logWithTime("INFO", "Stopping STUN server")
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			logWithTime("ERROR", "Error closing UDP listener: %v", err)
		}
	}
	if s.healthListener != nil {
		if err := s.healthListener.Close(); err != nil {
			logWithTime("ERROR", "Error closing health check listener: %v", err)
		}
	}
	logWithTime("INFO", "STUN server stopped")
}
