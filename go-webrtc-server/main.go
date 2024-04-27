package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	audioSrcFlag := fmt.Sprintf("audio-src-%s", id)
	audioSrcConfig := fmt.Sprintf("jackaudiosrc name=%s ! audioconvert ! audioresample", id)

	appSession.AudioSrc = flag.String(audioSrcFlag, audioSrcConfig, "GStreamer audio src")

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

var synthDefDirectory = "/app/supercollider/synthdefs"

func main() {
	signalChannel := make(chan os.Signal, 1)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")

		// revisit...
		// need all appSessions to do this gracefully
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

	appSession, ok := getOrCreateSession(r, w)
	if !ok {
		// If getOrCreateSession returned false, an error has already been sent to the client
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

func getOrCreateSession(r *http.Request, w http.ResponseWriter) (*AppSession, bool) {
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
	flag.Parse()

	var err error
	appSession, ok := getOrCreateSession(r, w)
	if !ok {
		// If getOrCreateSession returned false, an error has already been sent to the client
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
		log.Println("Creating pipeline...")

		appSession.GStreamerPipeline = gst.CreatePipeline(
			"opus",
			[]*webrtc.TrackLocalStaticSample{audioTrack},
			*appSession.AudioSrc,
		)
		appSession.GStreamerPipeline.Start()

		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	// Wait for the pipeline to be ready
	<-pipelineReady

	log.Println("Starting SuperCollider...")
	go startSuperCollider(appSession)

	<-gatherComplete

	sendPlaySynthMessage(appSession.SuperColliderPort)

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

func sendPlaySynthMessage(port int) {
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

func startSuperCollider(appSession *AppSession) {
	scPort, err := findAvailableSuperColliderPort()
	if err != nil {
		log.Printf("Error finding SuperCollider port: %v", err)
		return
	}
	appSession.SuperColliderPort = scPort

	gstJackPorts, err := setGStreamerJackPorts(appSession)
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

// TODO find a better way to set ports from jack_lsp
func setGStreamerJackPorts(appSession *AppSession) ([]string, error) {
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

// TODO find a deterministic way to sendPlaySynthMessage when SC is ready
func monitorSCSynthOutput(logFile *os.File, port int) {
	scanner := bufio.NewScanner(logFile)
	for scanner.Scan() {
		line := scanner.Text()
		log.Println("SCSynth Log:", line)
		if strings.Contains(line, "SuperCollider 3 server ready.") {
			sendPlaySynthMessage(port)
			break
		}
	}
}

// func connectJackPorts(appSession *AppSession) error {
// 	webrtcPorts, err := setGStreamerJackPorts(appSession)
// 	if err != nil {
// 		return fmt.Errorf("error finding JACK ports: %w", err)
// 	}

// 	var connectErrors []string
// 	for _, webrtcPort := range webrtcPorts {
// 		if err := connectPort("SuperCollider:out_1", webrtcPort); err != nil {
// 			connectErrors = append(connectErrors, err.Error())
// 		}
// 	}

// 	if len(connectErrors) > 0 {
// 		return fmt.Errorf("failed to connect some ports: %s", strings.Join(connectErrors, "; "))
// 	}

// 	return nil
// }

// func connectPort(outputPort, inputPort string) error {
// 	cmd := exec.Command("jack_connect", outputPort, inputPort)
// 	if err := cmd.Run(); err != nil {
// 		return fmt.Errorf("failed to connect %s to %s: %w", outputPort, inputPort, err)
// 	}
// 	return nil
// }

// STOP LOGIC /////////////////////////////////////////////////////////
func disconnectJackPorts(appSession *AppSession) error {
	webrtcPorts, err := setGStreamerJackPorts(appSession)
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
		appSession.PeerConnection = nil
	}

	if appSession.GStreamerPipeline != nil {
		appSession.GStreamerPipeline.Stop()
		appSession.GStreamerPipeline = nil
	}

	fmt.Println("All processes have been stopped.")
}

// Function to stop SuperCollider gracefully
func stopSuperCollider(appSession *AppSession) error {
	if appSession.SuperColliderCmd == nil || appSession.SuperColliderCmd.Process == nil {
		fmt.Println("SuperCollider is not running")
		return nil
	}

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
