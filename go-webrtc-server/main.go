package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"

	"github.com/gorilla/mux"
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

func main() {

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
	router.HandleFunc("/", serveHome).Methods("GET")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./client/")))
	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./client/index.html")
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	var browserOffer BrowserOffer
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
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
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

	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", state.String())
	})

	if err != nil {
		http.Error(w, "Failed to create peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion1")

	if err != nil {
		log.Printf("Failed to create audio track: %v\n", err)
		http.Error(w, "Failed to create audio track: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		log.Printf("Failed to add audio track to the peer connection: %v\n", err)
		http.Error(w, "Failed to add audio track to the peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Println("set remote description error:", err)
		http.Error(w, "Failed to set remote description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Println("Error creating answer:", err)
		http.Error(w, "Failed to create answer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Println("Error setting local description:", err)
		http.Error(w, "Failed to set local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audioSrc := flag.String("audio-src", "jackaudiosrc ! audioconvert ! audioresample", "GStreamer audio src")
	pipelineReady := make(chan struct{})

	go func() {
		log.Println("creating pipeline...")
		pipeline := gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, *audioSrc)
		pipeline.Start()

		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	// Wait for the pipeline to be ready
	<-pipelineReady

	log.Println("starting supercollider...")
	go startSuperCollider()

	<-gatherComplete

	localDescription := peerConnection.LocalDescription()
	encodedLocalDesc, err := json.Marshal(localDescription)
	if err != nil {
		log.Println("Error encoding local description:", err)
		http.Error(w, "Failed to encode local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encodedLocalDesc) // This writes the JSON representation of the local description
}

func startSuperCollider() {
	cmd := exec.Command("xvfb-run", "-a", "sclang", "/app/supercollider/liljedahl.scd")

	// Create pipes for stdout and stderr
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

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start SuperCollider: %v\n", err)
		return
	}
	log.Println("SuperCollider started")

	// Create scanner to read stdout and stderr
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Println("SuperCollider stdout: ", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Println("SuperCollider stderr: ", scanner.Text())
		}
	}()

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		log.Printf("SuperCollider exited with error: %v\n", err)
	} else {
		log.Println("SuperCollider finished successfully")
	}
}
