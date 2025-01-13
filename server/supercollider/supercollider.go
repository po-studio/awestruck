package supercollider

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hypebeast/go-osc/osc"
	"github.com/po-studio/server/jack"
	"github.com/po-studio/server/utils"
)

type SuperColliderSynth struct {
	Id             string
	Cmd            *exec.Cmd
	Port           int
	LogFile        *os.File
	GStreamerPorts string
	JackClientName string
	outputReader   *io.PipeReader
	OnClientName   func(string)
	ActiveSynthId  string
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

	// Create a pipe for reading scsynth's output
	pipeReader, pipeWriter := io.Pipe()
	s.outputReader = pipeReader

	// why we need these scsynth settings:
	// - bind to all interfaces for ECS networking
	// - optimize for container environment
	// - ensure stable audio processing
	s.Cmd = exec.Command(
		"scsynth",
		"-u", strconv.Itoa(s.Port),
		"-H", "0.0.0.0", // Listen on all interfaces
		"-a", "1024", // Audio bus channels
		"-i", "0", // Input channels
		"-o", "2", // Output channels
		"-b", "1026", // Number of buffers
		"-R", "0", // Real-time memory size
		"-C", "0", // Control bus channels
		"-l", "1", // Max logins
		"-z", "16", // Block size (helps with network jitter)
		"-P", "70", // Real-time priority
		"-V", "0", // Verbosity level
	)

	s.Cmd.Stdout = io.MultiWriter(logFile, pipeWriter)
	s.Cmd.Stderr = io.MultiWriter(logFile, pipeWriter)

	// why we need these environment variables:
	// - ensure proper synthdef loading
	// - set up jack client name
	// - configure network settings
	s.Cmd.Env = append(os.Environ(),
		"SC_SYNTHDEF_PATH="+utils.SCSynthDefDirectory,
		"JACK_START_SERVER=false",
		"JACK_NO_START_SERVER=true",
	)

	return nil
}

// Stop stops the SuperCollider server gracefully
func (s *SuperColliderSynth) Stop() error {
	log.Printf("[SCSYNTH][%s] Starting cleanup sequence", s.Id)

	// First disconnect JACK ports
	if err := jack.DisconnectJackPorts(s.Id, s.JackClientName); err != nil {
		log.Printf("[SCSYNTH][%s] Warning: error disconnecting JACK ports: %v", s.Id, err)
	} else {
		log.Printf("[SCSYNTH][%s] Successfully disconnected JACK ports", s.Id)
	}

	// Send quit message to scsynth
	client := osc.NewClient("127.0.0.1", s.Port)
	if err := client.Send(osc.NewMessage("/quit")); err != nil {
		log.Printf("[SCSYNTH][%s] Failed to send quit message: %v", s.Id, err)
	} else {
		log.Printf("[SCSYNTH][%s] Successfully sent quit message", s.Id)
	}

	// Close the output reader
	if s.outputReader != nil {
		if err := s.outputReader.Close(); err != nil {
			log.Printf("[SCSYNTH][%s] Error closing output reader: %v", s.Id, err)
		} else {
			log.Printf("[SCSYNTH][%s] Successfully closed output reader", s.Id)
		}
	}

	// Close log file
	if s.LogFile != nil {
		if err := s.LogFile.Close(); err != nil {
			log.Printf("[SCSYNTH][%s] Error closing log file: %v", s.Id, err)
		} else {
			log.Printf("[SCSYNTH][%s] Successfully closed log file", s.Id)
		}
	}

	// Kill the scsynth process if it's still running
	if s.Cmd != nil && s.Cmd.Process != nil {
		if err := s.Cmd.Process.Kill(); err != nil {
			log.Printf("[SCSYNTH][%s] Failed to kill process: %v", s.Id, err)
			return fmt.Errorf("failed to kill scsynth process: %w", err)
		}
		log.Printf("[SCSYNTH][%s] Successfully killed process", s.Id)
	}

	log.Printf("[SCSYNTH][%s] Cleanup sequence completed", s.Id)
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

	s.ActiveSynthId = synthDefName

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

func (s *SuperColliderSynth) waitForSuperColliderReady() error {
	client := osc.NewClient("127.0.0.1", s.Port)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Read scsynth output to find JACK client name
	scanner := bufio.NewScanner(s.outputReader)
	clientNameChan := make(chan string, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			// HACK: This is a hack to get the client name from the scsynth output
			// TODO: Find a better way to do this, but scsynth doesn't offer a clean
			// way to get this with JACK
			if strings.Contains(line, "JackDriver: client name is") {
				parts := strings.Split(line, "'")
				if len(parts) >= 2 {
					clientName := parts[1]
					clientNameChan <- clientName
					return
				}
			}
		}
	}()

	// Wait for both client name and server readiness
	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for SuperCollider to initialize")
		case clientName := <-clientNameChan:
			s.JackClientName = clientName
			if s.OnClientName != nil {
				s.OnClientName(clientName)
			}
		case <-ticker.C:
			msg := osc.NewMessage("/status")
			if err := client.Send(msg); err == nil && s.JackClientName != "" {
				return nil
			}
		}
	}
}

func (s *SuperColliderSynth) waitForJackPorts() error {
	portsChan := make(chan []string)
	errChan := make(chan error)
	timeout := time.After(10 * time.Second)

	expectedPort1 := fmt.Sprintf("%s:out_1", s.JackClientName)
	expectedPort2 := fmt.Sprintf("%s:out_2", s.JackClientName)

	go func() {
		for {
			cmd := exec.Command("jack_lsp")
			output, err := cmd.CombinedOutput()
			if err != nil {
				errChan <- fmt.Errorf("error checking JACK ports: %v", err)
				return
			}

			if strings.Contains(string(output), expectedPort1) {
				portsChan <- []string{expectedPort1, expectedPort2}
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
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list JACK connections: %v", err)
	}

	// Match either "webrtc-server" or "webrtc-server-<number>"
	re := regexp.MustCompile(`(webrtc-server(?:-\d+)?):in_` + regexp.QuoteMeta(s.Id))
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		log.Printf("Available JACK ports:\n%s", string(output))
		return fmt.Errorf("could not find webrtc-server ports for session %s", s.Id)
	}
	webrtcClientName := matches[1]
	log.Printf("Found WebRTC client name: %s", webrtcClientName)

	for i, scPort := range scPorts {
		gstPort := fmt.Sprintf("%s:in_%s_%d", webrtcClientName, s.Id, i+1)
		cmd = exec.Command("jack_connect", scPort, gstPort)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to connect %s to %s: %v", scPort, gstPort, err)
		}
		log.Printf("Successfully connected %s to %s", scPort, gstPort)
	}

	return nil
}

func (s *SuperColliderSynth) SetOnClientName(callback func(string)) {
	s.OnClientName = callback
}

// GetSynthCode returns the SuperCollider code for this synth
func (s *SuperColliderSynth) GetSynthCode() (string, error) {
	if s.ActiveSynthId == "" {
		return "", fmt.Errorf("no active synth")
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	// Extract identifier from synth ID
	parts := strings.Split(s.ActiveSynthId, "-")
	var searchPattern string
	if len(parts) >= 4 {
		// OpenAI format with timestamp
		timestamp := parts[len(parts)-1]
		searchPattern = timestamp
	} else {
		// Human format (e.g., romero_1)
		searchPattern = s.ActiveSynthId
	}

	// Find the matching .scd file
	srcDir := filepath.Join(cwd, "supercollider", "src")
	var matchingFile string
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".scd" &&
			(strings.Contains(path, searchPattern) || strings.Contains(info.Name(), searchPattern)) {
			matchingFile = path
			return filepath.SkipDir
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to find synth code: %v", err)
	}

	if matchingFile == "" {
		return "", fmt.Errorf("no matching synth code found for ID: %s", s.ActiveSynthId)
	}

	// Read the file
	code, err := os.ReadFile(matchingFile)
	if err != nil {
		return "", fmt.Errorf("failed to read synth code: %v", err)
	}

	return string(code), nil
}
