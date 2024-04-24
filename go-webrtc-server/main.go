package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/hypebeast/go-osc/osc"
	"github.com/pion/ice/v2" // Make sure the version is compatible with your webrtc version
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"

	osSignal "os/signal"

	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/internal/signal"
)

type BrowserOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

// AppState holds the state of the application
type AppState struct {
	PeerConnection    *webrtc.PeerConnection
	GStreamerPipeline *gst.Pipeline
	SuperColliderCmd  *exec.Cmd
}

// Global variable to hold the application state
var appState AppState
var audioSrc = flag.String("audio-src", "jackaudiosrc ! audioconvert ! audioresample", "GStreamer audio src")

func main() {
	flag.Parse()

	// Handle SIGINT and SIGTERM signals
	signalChannel := make(chan os.Signal, 1)
	osSignal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")

		// TODO perform cleanup here
		// Stop the running processes gracefully
		// Stop any WebRTC-related processes
		// Stop any GStreamer pipelines
		// Stop SuperCollider or any other audio processing tasks

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

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./client/index.html")
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	// Insert logic to stop streaming and clean up
	fmt.Println("Stop signal received, cleaning up...")
	// Assuming you have functions to stop processes
	stopAllProcesses()
	w.WriteHeader(http.StatusOK)
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	var browserOffer BrowserOffer
	var err error

	if err := json.NewDecoder(r.Body).Decode(&browserOffer); err != nil {
		log.Println("Error decoding offer:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	// Create a logger factory with the desired log level
	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.DefaultLogLevel = logging.LogLevelTrace

	// Create a setting engine and apply the logger factory
	settingEngine := webrtc.SettingEngine{
		LoggerFactory: loggerFactory,
	}
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	// Create the media engine
	mediaEngine := webrtc.MediaEngine{}
	// Ensure default codecs are registered
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Failed to register default codecs: %v", err)
	}

	// Create the API object with the setting engine and media engine
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
		webrtc.WithMediaEngine(&mediaEngine),
	)

	// Now, use this API instance to create your PeerConnection
	appState.PeerConnection, err = api.NewPeerConnection(webrtc.Configuration{
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

	appState.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", state.String())
	})

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion1")

	if err != nil {
		log.Printf("Failed to create audio track: %v\n", err)
		http.Error(w, "Failed to create audio track: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = appState.PeerConnection.AddTrack(audioTrack)
	if err != nil {
		log.Printf("Failed to add audio track to the peer connection: %v\n", err)
		http.Error(w, "Failed to add audio track to the peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = appState.PeerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Println("set remote description error:", err)
		http.Error(w, "Failed to set remote description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	answer, err := appState.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Println("Error creating answer:", err)
		http.Error(w, "Failed to create answer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(appState.PeerConnection)

	err = appState.PeerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Println("Error setting local description:", err)
		http.Error(w, "Failed to set local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// audioSrc := flag.String("audio-src", "jackaudiosrc ! audioconvert ! audioresample", "GStreamer audio src")
	pipelineReady := make(chan struct{})

	go func() {
		log.Println("creating pipeline...")
		appState.GStreamerPipeline = gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, *audioSrc)
		appState.GStreamerPipeline.Start()

		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	// Wait for the pipeline to be ready
	<-pipelineReady

	log.Println("starting supercollider...")
	go startSuperCollider()

	<-gatherComplete

	localDescription := appState.PeerConnection.LocalDescription()
	encodedLocalDesc, err := json.Marshal(localDescription)
	if err != nil {
		log.Println("Error encoding local description:", err)
		http.Error(w, "Failed to encode local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encodedLocalDesc) // This writes the JSON representation of the local description
}

func getRandomSCDFile(dir string) (string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var scdFiles []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".scd" {
			scdFiles = append(scdFiles, filepath.Join(dir, file.Name()))
		}
	}

	if len(scdFiles) == 0 {
		return "", fmt.Errorf("no .scd files found in %s", dir)
	}

	rand.Seed(time.Now().UnixNano())
	return scdFiles[rand.Intn(len(scdFiles))], nil
}

func startSuperCollider() {
	scdFile, err := getRandomSCDFile("/app/supercollider")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Set the command with the random .scd file
	log.Printf("SC FILE: %v\n", scdFile)
	appState.SuperColliderCmd = exec.Command("xvfb-run", "-a", "sclang", scdFile)

	stdout, err := appState.SuperColliderCmd.StdoutPipe()
	if err != nil {
		log.Printf("Error obtaining stdout: %v\n", err)
		return
	}
	stderr, err := appState.SuperColliderCmd.StderrPipe()
	if err != nil {
		log.Printf("Error obtaining stderr: %v\n", err)
		return
	}

	if err := appState.SuperColliderCmd.Start(); err != nil {
		log.Printf("Failed to start SuperCollider: %v\n", err)
		return
	}
	log.Println("SuperCollider command started")

	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			text := scanner.Text()
			log.Println("SuperCollider stdout: ", text)
			// if strings.Contains(text, "SUPERCOLLIDER_READY_FOR_JACK_CONNECTIONS") {
			if strings.Contains(text, "JackDriver: connected  SuperCollider:out_2 to system:playback_2") {
				log.Println("SuperCollider is ready. Connecting JACK ports...")
				if err := connectJackPorts(); err != nil {
					log.Printf("Error connecting JACK ports: %v\n", err)
				}
				break // Exit the loop after triggering connection
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Println("SuperCollider stderr: ", scanner.Text())
		}
	}()

	// Wait for the command to finish
	err = appState.SuperColliderCmd.Wait()
	if err != nil {
		log.Printf("SuperCollider exited with error: %v\n", err)
	} else {
		log.Println("SuperCollider finished successfully")
	}
}

func findJackPorts() ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var webrtcPorts []string
	for _, port := range ports {
		if strings.Contains(port, "webrtc-server:in_jackaudiosrc") {
			webrtcPorts = append(webrtcPorts, port)
		}
	}

	return webrtcPorts, nil
}

func connectJackPorts() error {
	webrtcPorts, err := findJackPorts()
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
	if err := sendOSCUpdate("/updateJACKOutputs", webrtcPorts); err != nil {
		return fmt.Errorf("failed to send OSC update for JACK ports: %w", err)
	}

	return nil
}

func connectPort(outputPort, inputPort string) error {
	cmd := exec.Command("jack_connect", outputPort, inputPort)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to connect %s to %s: %w", outputPort, inputPort, err)
	}
	return nil
}

func disconnectJackPorts() error {
	webrtcPorts, err := findJackPorts()
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

func stopAllProcesses() {
	fmt.Println("Stopping all processes...")

	stopSuperCollider()

	if err := disconnectJackPorts(); err != nil {
		fmt.Println("Error disconnecting JACK ports:", err)
	} else {
		fmt.Println("JACK ports disconnected successfully.")
	}

	if appState.PeerConnection != nil {
		if err := appState.PeerConnection.Close(); err != nil {
			fmt.Println("Error closing peer connection:", err)
		} else {
			fmt.Println("Peer connection closed successfully.")
		}
		appState.PeerConnection = nil // Ensure reference is released
	}

	if appState.GStreamerPipeline != nil {
		appState.GStreamerPipeline.Stop()
		appState.GStreamerPipeline = nil // Ensure reference is released
	}

	fmt.Println("All processes have been stopped.")
}

// Function to stop SuperCollider gracefully
func stopSuperCollider() error {
	if appState.SuperColliderCmd == nil || appState.SuperColliderCmd.Process == nil {
		fmt.Println("SuperCollider is not running")
		return nil // Or appropriate error
	}

	// Create OSC client and send /quit command
	client := osc.NewClient("localhost", 57110) // Change port if different
	msg := osc.NewMessage("/quit")
	err := client.Send(msg)
	if err != nil {
		fmt.Printf("Error sending OSC /quit message: %v\n", err)
		return err
	}
	fmt.Println("OSC /quit message sent successfully.")

	// Optionally, wait for the process to finish
	err = appState.SuperColliderCmd.Wait()
	if err != nil {
		fmt.Printf("Error waiting for SuperCollider to quit: %v\n", err)
		return err
	}

	fmt.Println("SuperCollider stopped gracefully.")
	return nil
}

func sendOSCMessage(address string) error {
	// Create a new OSC client
	client := osc.NewClient("localhost", 57110)

	// Create an OSC message with the specified address
	msg := osc.NewMessage(address)

	// Send the OSC message to SuperCollider
	err := client.Send(msg)
	if err != nil {
		fmt.Println("Error sending OSC message:", err)
		return err
	}

	fmt.Println("OSC message sent successfully.")
	return nil
}

func sendOSCUpdate(address string, ports []string) error {
	client := osc.NewClient("localhost", 57110)
	msg := osc.NewMessage(address)
	for _, port := range ports {
		msg.Append(port)
	}
	if err := client.Send(msg); err != nil {
		return fmt.Errorf("error sending OSC message: %w", err)
	}
	fmt.Println("OSC message sent successfully with ports update.")
	return nil
}
