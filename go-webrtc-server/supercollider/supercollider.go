package synth

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
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
	JackClientName string
	outputReader   *io.PipeReader
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

	s.Cmd = exec.Command(
		"scsynth",
		"-u", strconv.Itoa(s.Port),
		"-a", "1024",
		"-i", "0",
		"-o", "2",
		"-b", "1026",
		"-R", "0",
		"-C", "0",
		"-l", "1",
	)

	s.Cmd.Stdout = io.MultiWriter(logFile, pipeWriter)
	s.Cmd.Stderr = io.MultiWriter(logFile, pipeWriter)

	s.Cmd.Env = append(os.Environ(),
		"SC_SYNTHDEF_PATH="+utils.SCSynthDefDirectory,
	)

	return nil
}

// Stop stops the SuperCollider server gracefully
func (s *SuperColliderSynth) Stop() error {
	// First disconnect JACK ports
	if err := jack.DisconnectJackPorts(s.Id); err != nil {
		return fmt.Errorf("failed to disconnect JACK ports: %w", err)
	}

	// Send quit message to scsynth
	client := osc.NewClient("127.0.0.1", s.Port)
	if err := client.Send(osc.NewMessage("/quit")); err != nil {
		return fmt.Errorf("failed to send quit message to scsynth: %w", err)
	}

	// Close the output reader
	if s.outputReader != nil {
		s.outputReader.Close()
	}

	// Close log file
	if s.LogFile != nil {
		s.LogFile.Close()
	}

	// Kill the scsynth process if it's still running
	if s.Cmd != nil && s.Cmd.Process != nil {
		if err := s.Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill scsynth process: %w", err)
		}
	}

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
