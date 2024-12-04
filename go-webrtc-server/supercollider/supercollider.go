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

	// Wait for GStreamer JACK ports to be available
	var gstJackPorts []string
	for retries := 0; retries < 5; retries++ {
		gstJackPorts, err = jack.GetGStreamerJackPorts(s.Id)
		if err == nil && len(gstJackPorts) > 0 {
			break
		}
		log.Printf("Waiting for GStreamer JACK ports (attempt %d/5)...", retries+1)
		time.Sleep(time.Second)
	}
	if err != nil {
		return fmt.Errorf("error finding GStreamer-JACK ports: %v", err)
	}
	s.GStreamerPorts = strings.Join(gstJackPorts, ",")

	if err := s.setupCmd(); err != nil {
		return err
	}

	if err := s.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start scsynth: %v", err)
	}
	log.Printf("scsynth started on port %d", s.Port)

	// Wait for SuperCollider to initialize
	time.Sleep(2 * time.Second)

	if err := s.monitorJackPorts(); err != nil {
		log.Printf("Warning: Failed to monitor JACK ports: %v", err)
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
