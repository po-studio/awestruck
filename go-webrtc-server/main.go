package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hypebeast/go-osc/osc"
	"github.com/pion/ice/v2"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"

	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/internal/signal"

	"github.com/po-studio/go-webrtc-server/session"
	sc "github.com/po-studio/go-webrtc-server/supercollider"
)

type BrowserOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

func main() {
	signalChannel := make(chan os.Signal, 1)

	go func() {
		<-signalChannel
		log.Println("Received shutdown signal. Stopping processes...")
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
	fmt.Println("Stop signal received, cleaning up...")

	appSession, ok := session.GetOrCreateSession(r, w)
	if !ok {
		return
	}

	stopAllProcesses(appSession)
	w.WriteHeader(http.StatusOK)
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	flag.Parse()

	var err error
	appSession, ok := session.GetOrCreateSession(r, w)
	if !ok {
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
	go sc.StartSuperCollider(appSession)

	<-gatherComplete

	sc.SendPlaySynthMessage(appSession.SuperColliderPort)

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

func disconnectJackPorts(appSession *session.AppSession) error {
	webrtcPorts, err := sc.SetGStreamerJackPorts(appSession)
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

func stopAllProcesses(appSession *session.AppSession) {
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

func stopSuperCollider(appSession *session.AppSession) error {
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
