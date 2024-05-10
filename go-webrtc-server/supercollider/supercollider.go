// supercollider.go
package synth

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hypebeast/go-osc/osc"
)

var synthDefDirectory = "/app/supercollider/synthdefs"

type SuperColliderSynth struct {
	Id             string
	Cmd            *exec.Cmd
	Port           int
	LogFile        *os.File
	GStreamerPorts string
}

func (s *SuperColliderSynth) GetPort() int {
	return s.Port
}

func (s *SuperColliderSynth) Start() error {
	port, err := findAvailableSuperColliderPort()
	if err != nil {
		return fmt.Errorf("error finding SuperCollider port: %v", err)
	}
	s.Port = port

	gstJackPorts, err := findGStreamerJackPorts(s.Id)
	if err != nil {
		return fmt.Errorf("error finding GStreamer-JACK ports: %v", err)
	}
	s.GStreamerPorts = strings.Join(gstJackPorts, ",")

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
	s.Cmd.Env = append(os.Environ(),
		"SC_JACK_DEFAULT_OUTPUTS="+s.GStreamerPorts,
		"SC_SYNTHDEF_PATH="+synthDefDirectory,
	)

	logFilePath := fmt.Sprintf("/app/scsynth_%s.log", s.Id)
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	s.LogFile = logFile
	s.Cmd.Stdout = logFile
	s.Cmd.Stderr = logFile

	if err := s.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start scsynth: %v", err)
	}
	log.Println("scsynth command started with dynamically assigned port:", s.Port)

	// Start monitoring the log file output for the server ready message
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitorSCSynthOutput(logFilePath, s.Port)
	}()
	wg.Wait()

	return nil
}

func (s *SuperColliderSynth) Stop() error {
	if s.Cmd == nil || s.Cmd.Process == nil {
		fmt.Println("SuperCollider is not running")
		return nil
	}

	client := osc.NewClient("localhost", s.Port)
	msg := osc.NewMessage("/quit")
	err := client.Send(msg)
	if err != nil {
		fmt.Printf("Error sending OSC /quit message: %v\n", err)
		return err
	}
	fmt.Println("OSC /quit message sent successfully.")

	err = s.Cmd.Wait()
	if err != nil {
		fmt.Printf("Error waiting for SuperCollider to quit: %v\n", err)
		return err
	}

	fmt.Println("SuperCollider stopped gracefully.")
	return nil
}

func monitorSCSynthOutput(logFilePath string, port int) {
	log.Println("Monitoring Synth with port:", port)

	// Open the log file in read-only mode
	logFile, err := os.Open(logFilePath)
	if err != nil {
		log.Printf("Error opening log file for monitoring: %v", err)
		return
	}
	defer logFile.Close()

	scanner := bufio.NewScanner(logFile)
	for scanner.Scan() {
		line := scanner.Text()
		log.Println("SCSynth Log:", line)
		if strings.Contains(line, "SuperCollider 3 server ready.") {
			SendPlaySynthMessage(port)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from log file: %v", err)
	}
}

func getRandomSynthDefName() (string, error) {
	files, err := os.ReadDir(synthDefDirectory)
	if err != nil {
		return "", err
	}

	var synthDefs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".scsyndef" {
			baseName := strings.TrimSuffix(file.Name(), ".scsyndef")
			synthDefs = append(synthDefs, baseName)
		}
	}

	if len(synthDefs) == 0 {
		return "", fmt.Errorf("no .scsyndef files found in %s", synthDefDirectory)
	}

	randTimeSeed := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(randTimeSeed)

	chosenFile := synthDefs[rnd.Intn(len(synthDefs))]

	return chosenFile, nil
}

func SendPlaySynthMessage(port int) {
	client := osc.NewClient("127.0.0.1", port)
	msg := osc.NewMessage("/s_new")

	synthDefName, err := getRandomSynthDefName()
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

func findAvailableSuperColliderPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port, nil
}

func findGStreamerJackPorts(id string) ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var gstJackPorts []string
	prefix := "webrtc-server"

	for _, port := range ports {
		if strings.HasPrefix(port, prefix) && strings.Contains(port, id) {
			gstJackPorts = append(gstJackPorts, port)
		}
	}

	return gstJackPorts, nil
}
