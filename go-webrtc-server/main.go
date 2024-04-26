package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/hypebeast/go-osc/osc"
	"github.com/pion/ice/v2"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"

	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/internal/signal"
)

type BrowserOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

type AppSession struct {
	Id                string
	PeerConnection    *webrtc.PeerConnection
	GStreamerPipeline *gst.Pipeline
	SuperColliderCmd  *exec.Cmd
	SuperColliderPort int
	AudioSrc          *string
}

type SessionManager struct {
	Sessions map[string]*AppSession
	mutex    sync.Mutex
}

func (sm *SessionManager) CreateSession(id string) *AppSession {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	appSession := &AppSession{}
	appSession.Id = id
	// audioSrcId := fmt.Sprintf("audio-src-%s", id)
	// audioSrcWithID := fmt.Sprintf("jackaudiosrc ! audioconvert ! audioresample", id)
	audioSrcWithID := "jackaudiosrc ! audioconvert ! audioresample"
	appSession.AudioSrc = flag.String("audio-src", audioSrcWithID, "GStreamer audio src")
	sm.Sessions[id] = appSession
	return appSession
}

func (sm *SessionManager) GetSession(id string) (*AppSession, bool) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	session, exists := sm.Sessions[id]
	return session, exists
}

func (sm *SessionManager) DeleteSession(id string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	delete(sm.Sessions, id)
}

var sessionManager = SessionManager{
	Sessions: make(map[string]*AppSession),
}

func main() {
	flag.Parse()

	signalChannel := make(chan os.Signal, 1)
	// osSignal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")

		// need all appSessions to do this gracefully
		// revisit...
		// stopAllProcesses()

		os.Exit(0)
	}()

	router := mux.NewRouter()
	router.HandleFunc("/offer", handleOffer).Methods("POST")
	router.HandleFunc("/stop", handleStop).Methods("POST")
	router.HandleFunc("/", serveHome).Methods("GET")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./client/")))
	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
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

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./client/index.html")
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Stop signal received, cleaning up...")

	appSession, ok := getAppSessionBySessionID(r, w)
	if !ok {
		// If getAppSessionBySessionID returned false, an error has already been sent to the client
		return
	}

	stopAllProcesses(appSession)
	w.WriteHeader(http.StatusOK)
}

func getSessionIDFromHeader(r *http.Request) (string, bool) {
	sessionID := r.Header.Get("X-Session-ID") // Custom header, typically prefixed with 'X-'
	if sessionID == "" {
		return "", false // No session ID found in headers
	}
	return sessionID, true
}

func getAppSessionBySessionID(r *http.Request, w http.ResponseWriter) (*AppSession, bool) {
	// Extract session ID from the HTTP header
	sessionID, ok := getSessionIDFromHeader(r)
	if !ok {
		log.Println("No session ID provided in the header")
		http.Error(w, "No session ID provided", http.StatusBadRequest)
		return nil, false
	}

	// Retrieve or create a session based on the session ID
	appSession, exists := sessionManager.GetSession(sessionID)
	if !exists {
		// Optionally create a new session if one does not exist
		appSession = sessionManager.CreateSession(sessionID)
	}

	return appSession, true
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	var err error

	appSession, ok := getAppSessionBySessionID(r, w)
	if !ok {
		// If getAppSessionBySessionID returned false, an error has already been sent to the client
		return
	}

	var browserOffer BrowserOffer
	if err := json.NewDecoder(r.Body).Decode(&browserOffer); err != nil {
		log.Println("Error decoding offer:", err)
		http.Error(w, "Invalid offer format", http.StatusBadRequest)
		return
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.DefaultLogLevel = logging.LogLevelTrace

	settingEngine := webrtc.SettingEngine{
		LoggerFactory: loggerFactory,
	}
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	mediaEngine := webrtc.MediaEngine{}

	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Failed to register default codecs: %v", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
		webrtc.WithMediaEngine(&mediaEngine),
	)

	appSession.PeerConnection, err = api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})

	if err != nil {
		log.Println("Failed to create peer connection:", err)
		http.Error(w, "Failed to create peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", state.String())
	})

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion1")

	if err != nil {
		log.Printf("Failed to create audio track: %v\n", err)
		http.Error(w, "Failed to create audio track: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = appSession.PeerConnection.AddTrack(audioTrack)
	if err != nil {
		log.Printf("Failed to add audio track to the peer connection: %v\n", err)
		http.Error(w, "Failed to add audio track to the peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = appSession.PeerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Println("set remote description error:", err)
		http.Error(w, "Failed to set remote description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	answer, err := appSession.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Println("Error creating answer:", err)
		http.Error(w, "Failed to create answer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(appSession.PeerConnection)

	err = appSession.PeerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Println("Error setting local description:", err)
		http.Error(w, "Failed to set local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pipelineReady := make(chan struct{})

	go func() {
		log.Println("creating pipeline...")
		appSession.GStreamerPipeline = gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, *appSession.AudioSrc)
		appSession.GStreamerPipeline.Start()

		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	// Wait for the pipeline to be ready
	<-pipelineReady

	log.Println("starting supercollider...")
	go startSuperCollider(appSession)

	<-gatherComplete

	localDescription := appSession.PeerConnection.LocalDescription()
	encodedLocalDesc, err := json.Marshal(localDescription)
	if err != nil {
		log.Println("Error encoding local description:", err)
		http.Error(w, "Failed to encode local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// This writes the JSON representation of the local description for the client
	w.Write(encodedLocalDesc)
}

func getRandomSynthDefName(dir string) (string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var synthDefs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".scsyndef" {
			// Trim the '.scsyndef' extension from the filename before adding to the list
			baseName := strings.TrimSuffix(file.Name(), ".scsyndef")
			synthDefs = append(synthDefs, baseName)
		}
	}

	if len(synthDefs) == 0 {
		return "", fmt.Errorf("no .scsyndef files found in %s", dir)
	}

	rand.Seed(time.Now().UnixNano())
	chosenFile := synthDefs[rand.Intn(len(synthDefs))]
	return chosenFile, nil
}

// func sendOSCUpdate(address string, jackGStreamerPorts []string, scsynthPort int) error {
// 	client := osc.NewClient("localhost", scsynthPort)
// 	msg := osc.NewMessage(address)
// 	for _, port := range jackGStreamerPorts {
// 		msg.Append(port)
// 	}
// 	if err := client.Send(msg); err != nil {
// 		return fmt.Errorf("error sending OSC message: %w", err)
// 	}
// 	fmt.Println("OSC message sent successfully with ports update.")
// 	return nil
// }

func sendPlaySynthMessage(port int, synthDefName string) {
	client := osc.NewClient("localhost", port)

	// Create and send the /s_new message to start the synth
	msg := osc.NewMessage("/s_new simpleTone 1 0 1")
	// msg.Append(synthDefName) // SynthDef name
	// msg.Append(int32(-1))    // Node ID, -1 lets the server choose the ID
	// msg.Append(int32(0))     // Position in the node tree, 0 for default group
	// msg.Append(int32(1))     // Add to the head of the group
	// Optionally append any initial parameters your SynthDef supports
	// e.g., msg.Append("freq", float32(440.0))

	log.Printf("SENDING MESSAGE: %v\n", msg)
	err := client.Send(msg)
	if err != nil {
		log.Printf("Error sending OSC message: %v\n", err)
		return
	}
	fmt.Println("Synth message sent successfully.")

	// Create and send the /synthdef/query message to query available SynthDefs
	queryMsg := osc.NewMessage("/synthdef/query")
	err = client.Send(queryMsg)
	if err != nil {
		log.Printf("Error sending synthdef query OSC message: %v\n", err)
		return
	}
	fmt.Println("Synthdef query message sent successfully.")
}

// func startSuperCollider(appSession *AppSession) {
// 	port := 57110 // Assuming static for example, you should dynamically find this as per your existing setup.
// 	appSession.SuperColliderPort = port

// 	synthDefDirectory := "/app/supercollider/synthdefs"
// 	jackPorts, err := findJackPorts(appSession)
// 	if err != nil {
// 		log.Printf("Error finding JACK ports: %v\n", err)
// 		return
// 	}
// 	jackPortsString := strings.Join(jackPorts, ",")

// 	cmd := exec.Command(
// 		"scsynth", // Command to run the SuperCollider server
// 		"-u", "57110",
// 		// "-l", "1", // Number of audio bus channels to allocate (control rate or audio rate)
// 		"-i", "0", // Number of audio input buses (stereo input)
// 		"-o", "2", // Number of audio output buses (stereo output)
// 	)

// 	cmd.Env = append(os.Environ(),
// 		"SC_JACK_DEFAULT_OUTPUTS="+jackPortsString,
// 		"SC_SYNTHDEF_PATH="+synthDefDirectory,
// 	)

// 	stdout, err := cmd.StdoutPipe()
// 	if err != nil {
// 		log.Printf("Error obtaining stdout: %v\n", err)
// 		return
// 	}
// 	stderr, err := cmd.StderrPipe()
// 	if err != nil {
// 		log.Printf("Error obtaining stderr: %v\n", err)
// 		return
// 	}

// 	if err := cmd.Start(); err != nil {
// 		log.Printf("Failed to start scsynth: %v\n", err)
// 		return
// 	}
// 	log.Println("scsynth command started with dynamically assigned port:", port)

// 	synthDef, err := getRandomSynthDefName(synthDefDirectory)
// 	if err != nil {
// 		log.Printf("Error obtaining synthdef: %v\n", err)
// 		return
// 	}
// 	sendPlaySynthMessage(port, synthDef)

// 	go handleSuperColliderOutput(stdout, stderr)
// }

func startSuperCollider(appSession *AppSession) {
	port := 57110 // Assuming static for example, you should dynamically find this as per your existing setup.
	appSession.SuperColliderPort = port

	synthDefDirectory := "/app/supercollider/synthdefs"
	jackPorts, err := findJackPorts(appSession)
	if err != nil {
		log.Printf("Error finding JACK ports: %v\n", err)
		return
	}
	jackPortsString := strings.Join(jackPorts, ",")

	cmd := exec.Command(
		"scsynth", // Command to run the SuperCollider server
		"-u", fmt.Sprintf("%d", port),
		"-i", "0", // Number of audio input buses (stereo input)
		"-o", "2", // Number of audio output buses (stereo output)
	)

	cmd.Env = append(os.Environ(),
		"SC_JACK_DEFAULT_OUTPUTS="+jackPortsString,
		"SC_SYNTHDEF_PATH="+synthDefDirectory,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error obtaining stdout: %v\n", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Error obtaining stderr: %v\n", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start scsynth: %v\n", err)
		return
	}
	log.Println("scsynth command started with dynamically assigned port:", port)

	// Wait for SuperCollider to report it's ready
	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			log.Println("STDOUT:", line)
			if strings.Contains(line, "SuperCollider 3 server ready.") {
				synthDef, err := getRandomSynthDefName(synthDefDirectory)
				if err != nil {
					log.Printf("Error obtaining synthdef: %v\n", err)
					return
				}
				sendPlaySynthMessage(port, synthDef)
				break
			}
		}
	}()

	errScanner := bufio.NewScanner(stderr)
	go func() {
		for errScanner.Scan() {
			log.Println("STDERR:", errScanner.Text())
		}
	}()
}

func handleSuperColliderOutput(stdout, stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			log.Println("STDOUT:", scanner.Text())
		}
	}()

	errScanner := bufio.NewScanner(stderr)
	go func() {
		for errScanner.Scan() {
			log.Println("STDERR:", errScanner.Text())
		}
	}()
}

func findJackPorts(appSession *AppSession) ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var webrtcPorts []string
	// searchString := fmt.Sprintf("webrtc-server:in_%s", appSession.Id) // Construct the search string dynamically
	searchString := fmt.Sprintf("webrtc-server:in_jackaudiosrc")
	for _, port := range ports {
		if strings.Contains(port, searchString) { // Use the dynamically constructed search string
			webrtcPorts = append(webrtcPorts, port)
		}
	}

	return webrtcPorts, nil
}

func connectJackPorts(appSession *AppSession) error {
	webrtcPorts, err := findJackPorts(appSession)
	if err != nil {
		return fmt.Errorf("error finding JACK ports: %w", err)
	}

	var connectErrors []string
	for _, webrtcPort := range webrtcPorts {
		if err := connectPort("SuperCollider:out_1", webrtcPort); err != nil {
			connectErrors = append(connectErrors, err.Error())
		}
	}

	if len(connectErrors) > 0 {
		return fmt.Errorf("failed to connect some ports: %s", strings.Join(connectErrors, "; "))
	}

	// Send the new port names to SuperCollider
	// See startup.scd, which defines the API for "/updateJACKOutputs"
	// if err := sendOSCUpdate("/updateJACKOutputs", webrtcPorts, appSession.SuperColliderPort); err != nil {
	// 	return fmt.Errorf("failed to send OSC update for JACK ports: %w", err)
	// }

	return nil
}

func connectPort(outputPort, inputPort string) error {
	cmd := exec.Command("jack_connect", outputPort, inputPort)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to connect %s to %s: %w", outputPort, inputPort, err)
	}
	return nil
}

func disconnectJackPorts(appSession *AppSession) error {
	webrtcPorts, err := findJackPorts(appSession)
	if err != nil {
		return fmt.Errorf("error finding JACK ports: %w", err)
	}

	var disconnectErrors []string
	for _, webrtcPort := range webrtcPorts {
		if err := disconnectPort("SuperCollider:out_1", webrtcPort); err != nil {
			disconnectErrors = append(disconnectErrors, err.Error())
		}
	}

	if len(disconnectErrors) > 0 {
		return fmt.Errorf("failed to disconnect some ports: %s", strings.Join(disconnectErrors, "; "))
	}
	return nil
}

func disconnectPort(outputPort, inputPort string) error {
	cmd := exec.Command("jack_disconnect", outputPort, inputPort)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disconnect %s from %s: %w", outputPort, inputPort, err)
	}
	return nil
}

func stopAllProcesses(appSession *AppSession) {
	fmt.Println("Stopping all processes...")

	stopSuperCollider(appSession)

	if err := disconnectJackPorts(appSession); err != nil {
		fmt.Println("Error disconnecting JACK ports:", err)
	} else {
		fmt.Println("JACK ports disconnected successfully.")
	}

	if appSession.PeerConnection != nil {
		if err := appSession.PeerConnection.Close(); err != nil {
			fmt.Println("Error closing peer connection:", err)
		} else {
			fmt.Println("Peer connection closed successfully.")
		}
		appSession.PeerConnection = nil // Ensure reference is released
	}

	if appSession.GStreamerPipeline != nil {
		appSession.GStreamerPipeline.Stop()
		appSession.GStreamerPipeline = nil // Ensure reference is released
	}

	fmt.Println("All processes have been stopped.")
}

// Function to stop SuperCollider gracefully
func stopSuperCollider(appSession *AppSession) error {
	if appSession.SuperColliderCmd == nil || appSession.SuperColliderCmd.Process == nil {
		fmt.Println("SuperCollider is not running")
		return nil // Or appropriate error
	}

	// Create OSC client and send /quit command
	// Change port if different

	// TODO specify SC ports as environment variables
	// we may need to use multiple ports
	client := osc.NewClient("localhost", appSession.SuperColliderPort)
	msg := osc.NewMessage("/quit")
	err := client.Send(msg)
	if err != nil {
		fmt.Printf("Error sending OSC /quit message: %v\n", err)
		return err
	}
	fmt.Println("OSC /quit message sent successfully.")

	// Optionally, wait for the process to finish
	err = appSession.SuperColliderCmd.Wait()
	if err != nil {
		fmt.Printf("Error waiting for SuperCollider to quit: %v\n", err)
		return err
	}

	fmt.Println("SuperCollider stopped gracefully.")
	return nil
}

func sendOSCUpdate(address string, jackGStreamerPorts []string, scsynthPort int) error {
	client := osc.NewClient("localhost", scsynthPort)
	msg := osc.NewMessage(address)
	for _, port := range jackGStreamerPorts {
		msg.Append(port)
	}
	if err := client.Send(msg); err != nil {
		return fmt.Errorf("error sending OSC message: %w", err)
	}
	fmt.Println("OSC message sent successfully with ports update.")
	return nil
}
