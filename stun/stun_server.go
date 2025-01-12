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

// helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
	logWithTime("INFO", "STUN server stopped")
}
