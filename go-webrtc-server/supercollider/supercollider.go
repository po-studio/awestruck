package synth

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hypebeast/go-osc/osc"
	"github.com/po-studio/go-webrtc-server/jack"
	"github.com/po-studio/go-webrtc-server/utils"
)

type SuperColliderSynth struct {
	Id             string
	Cmd            *exec.Cmd
	Port           int
	LogFile        *os.File
	GStreamerPorts string
}

const (
	DefaultSuperColliderLogDir = "/app"
)

// NewSuperColliderSynth creates and returns a new SuperColliderSynth instance
func NewSuperColliderSynth(id string) *SuperColliderSynth {
	return &SuperColliderSynth{Id: id}
}

// GetPort returns the port number assigned to the SuperCollider instance
func (s *SuperColliderSynth) GetPort() int {
	return s.Port
}

// Start initializes and starts the SuperCollider server
func (s *SuperColliderSynth) Start() error {
	port, err := utils.FindAvailablePort()
	if err != nil {
		return fmt.Errorf("error finding SuperCollider port: %v", err)
	}
	s.Port = port

	// First ensure GStreamer pipeline is ready
	gstPortsChan := make(chan []string)
	gstErrChan := make(chan error)
	timeout := time.After(10 * time.Second)

	go func() {
		for {
			ports, err := jack.GetGStreamerJackPorts(s.Id)
			if err != nil {
				gstErrChan <- err
				return
			}
			if len(ports) > 0 {
				gstPortsChan <- ports
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Wait for GStreamer ports
	var gstJackPorts []string
	select {
	case ports := <-gstPortsChan:
		gstJackPorts = ports
	case err := <-gstErrChan:
		return fmt.Errorf("error finding GStreamer-JACK ports: %v", err)
	case <-timeout:
		return fmt.Errorf("timeout waiting for GStreamer-JACK ports")
	}

	s.GStreamerPorts = strings.Join(gstJackPorts, ",")

	// Now start SuperCollider
	if err := s.setupCmd(); err != nil {
		return err
	}

	if err := s.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start scsynth: %v", err)
	}
	log.Printf("scsynth started on port %d", s.Port)

	// Wait for SuperCollider to be ready
	if err := s.waitForSuperColliderReady(); err != nil {
		return fmt.Errorf("SuperCollider failed to initialize: %v", err)
	}

	// Finally connect the ports
	if err := s.waitForJackPorts(); err != nil {
		return fmt.Errorf("failed to setup JACK connections: %v", err)
	}

	return nil
}

// setupCmd prepares the scsynth command with the appropriate arguments and environment variables
func (s *SuperColliderSynth) setupCmd() error {
	logFilePath := fmt.Sprintf("%s/scsynth_%s.log", DefaultSuperColliderLogDir, s.Id)
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	s.LogFile = logFile

	s.Cmd = exec.Command(
		"scsynth",
		"-u", strconv.Itoa(s.Port), // UDP port for OSC communication (SuperCollider's Open Sound Control server)
		"-a", "1024", // Maximum number of audio bus channels
		"-i", "0", // Number of input bus channels (0 to disable audio input)
		"-o", "2", // Number of output bus channels (e.g., 2 for stereo output)
		"-b", "1026", // Number of audio buffer frames (used for sound synthesis)
		"-R", "0", // Real-time memory lock (0 to disable, 1 to enable)
		"-C", "0", // Hardware control bus channels (0 to disable control buses)
		"-l", "1", // Maximum log level (1 for errors only, 3 for full logs)
	)

	s.Cmd.Env = append(os.Environ(),
		"SC_JACK_DEFAULT_OUTPUTS="+s.GStreamerPorts,
		"SC_SYNTHDEF_PATH="+utils.SCSynthDefDirectory,
	)
	s.Cmd.Stdout = s.LogFile
	s.Cmd.Stderr = s.LogFile
	return nil
}

// Stop stops the SuperCollider server gracefully
func (s *SuperColliderSynth) Stop() error {
	if s.Cmd == nil || s.Cmd.Process == nil {
		log.Println("SuperCollider is not running")
		return nil
	}

	client := osc.NewClient("127.0.0.1", s.Port)
	msg := osc.NewMessage("/quit")
	if err := client.Send(msg); err != nil {
		log.Printf("Error sending OSC /quit message: %v\n", err)
		return err
	}
	log.Println("OSC /quit message sent successfully.")

	if err := s.Cmd.Wait(); err != nil {
		log.Printf("Error waiting for SuperCollider to quit: %v\n", err)
		return err
	}

	log.Println("SuperCollider stopped gracefully.")
	return nil
}

// SendPlayMessage sends an OSC message to the SuperCollider server to play a random synth
func (s *SuperColliderSynth) SendPlayMessage() {
	client := osc.NewClient("127.0.0.1", s.Port)
	msg := osc.NewMessage("/s_new")

	synthDefName, err := utils.GetRandomSynthDefName()
	if err != nil {
		log.Printf("Could not find synthdef name: %v", err)
		return
	}

	msg.Append(synthDefName)
	msg.Append(int32(1)) // node ID
	msg.Append(int32(0)) // action: 0 for add to head
	msg.Append(int32(0)) // target group ID

	log.Printf("Sending OSC message: %v", msg)
	if err := client.Send(msg); err != nil {
		log.Printf("Error sending OSC message: %v\n", err)
	} else {
		log.Println("OSC message sent successfully.")
	}
}

// New function to monitor JACK ports
func (s *SuperColliderSynth) monitorJackPorts() error {
	// First list all connections
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list JACK connections: %v", err)
	}
	log.Printf("JACK Connections:\n%s", string(output))

	// Wait for SuperCollider ports to appear
	var scPorts []string
	for retries := 0; retries < 5; retries++ {
		cmd := exec.Command("jack_lsp")
		output, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(output), "SuperCollider:out_1") {
			scPorts = []string{"SuperCollider:out_1", "SuperCollider:out_2"}
			break
		}
		log.Printf("Waiting for SuperCollider ports (attempt %d/5)...", retries+1)
		time.Sleep(time.Second)
	}

	if len(scPorts) == 0 {
		return fmt.Errorf("SuperCollider ports not found")
	}

	// Connect SuperCollider outputs to GStreamer inputs
	for i, scPort := range scPorts {
		gstPort := fmt.Sprintf("%s:in_jackaudiosrc0_%d", s.Id, i+1)
		connectCmd := exec.Command("jack_connect", scPort, gstPort)
		if err := connectCmd.Run(); err != nil {
			log.Printf("Warning: Failed to connect %s to %s: %v", scPort, gstPort, err)
		} else {
			log.Printf("Successfully connected %s to %s", scPort, gstPort)
		}
	}

	return nil
}

func (s *SuperColliderSynth) waitForSuperColliderReady() error {
	client := osc.NewClient("127.0.0.1", s.Port)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for SuperCollider to initialize")
		case <-ticker.C:
			// Send /status message to check if server is ready
			msg := osc.NewMessage("/status")
			if err := client.Send(msg); err == nil {
				return nil
			}
		}
	}
}

func (s *SuperColliderSynth) waitForJackPorts() error {
	portsChan := make(chan []string)
	errChan := make(chan error)
	timeout := time.After(10 * time.Second)

	go func() {
		for {
			cmd := exec.Command("jack_lsp")
			output, err := cmd.CombinedOutput()
			if err != nil {
				errChan <- fmt.Errorf("error checking JACK ports: %v", err)
				return
			}

			if strings.Contains(string(output), "SuperCollider:out_1") {
				portsChan <- []string{"SuperCollider:out_1", "SuperCollider:out_2"}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case ports := <-portsChan:
		return s.connectJackPorts(ports)
	case err := <-errChan:
		return err
	case <-timeout:
		return fmt.Errorf("timeout waiting for JACK ports")
	}
}

func (s *SuperColliderSynth) connectJackPorts(scPorts []string) error {
	// First get current connections
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list JACK connections: %v", err)
	}
	connections := string(output)

	for i, scPort := range scPorts {
		gstPort := fmt.Sprintf("webrtc-server:in_%s_%d", s.Id, i+1)

		// Check if connection already exists
		connectionString := fmt.Sprintf("%s\n   %s", scPort, gstPort)
		if strings.Contains(connections, connectionString) {
			log.Printf("Ports already connected: %s -> %s", scPort, gstPort)
			continue
		}

		// Try to connect the ports
		connectCmd := exec.Command("jack_connect", scPort, gstPort)
		if err := connectCmd.Run(); err != nil {
			return fmt.Errorf("failed to connect %s to %s: %v", scPort, gstPort, err)
		}
		log.Printf("Successfully connected %s to %s", scPort, gstPort)
	}
	return nil
}
