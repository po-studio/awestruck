package supercollider

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hypebeast/go-osc/osc"
	"github.com/po-studio/go-webrtc-server/session"
)

var synthDefDirectory = "/app/supercollider/synthdefs"

func StartSuperCollider(appSession *session.AppSession) {
	scPort, err := findAvailableSuperColliderPort()
	if err != nil {
		log.Printf("Error finding SuperCollider port: %v", err)
		return
	}
	appSession.SuperColliderPort = scPort

	gstJackPorts, err := SetGStreamerJackPorts(appSession)
	if err != nil {
		log.Printf("Error finding GStreamer-JACK ports: %v", err)
		return
	}
	gstJackPortsStr := strings.Join(gstJackPorts, ",")

	cmd := exec.Command(
		"scsynth",
		"-u", strconv.Itoa(scPort),
		"-a", "1024",
		"-i", "2",
		"-o", "2",
		"-b", "1026",
		"-R", "0",
		"-C", "0",
		"-l", "1",
	)
	cmd.Env = append(os.Environ(),
		"SC_JACK_DEFAULT_OUTPUTS="+gstJackPortsStr,
		"SC_SYNTHDEF_PATH="+synthDefDirectory,
	)

	// Open a file to log scsynth output
	logFile, err := os.OpenFile("/app/scsynth_"+appSession.Id+".log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start scsynth: %v", err)
		return
	}
	appSession.SuperColliderCmd = cmd
	log.Println("scsynth command started with dynamically assigned port:", scPort)

	go monitorSCSynthOutput(logFile, appSession.SuperColliderPort)
}

func getRandomSynthDefName() (string, error) {
	files, err := os.ReadDir(synthDefDirectory)
	if err != nil {
		return "", err
	}

	var synthDefs []string
	for _, file := range files {

		// Trim the '.scsyndef' extension from the filename before adding to the list
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

func monitorSCSynthOutput(logFile *os.File, port int) {
	scanner := bufio.NewScanner(logFile)
	for scanner.Scan() {
		line := scanner.Text()
		log.Println("SCSynth Log:", line)
		if strings.Contains(line, "SuperCollider 3 server ready.") {
			SendPlaySynthMessage(port)
			break
		}
	}
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

func SetGStreamerJackPorts(appSession *session.AppSession) ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var gstJackPorts []string
	prefix := "webrtc-server"

	log.Println("appSession.Id: ", appSession.Id)
	for _, port := range ports {
		if strings.HasPrefix(port, prefix) && strings.Contains(port, appSession.Id) {
			gstJackPorts = append(gstJackPorts, port)
		}
	}

	return gstJackPorts, nil
}
