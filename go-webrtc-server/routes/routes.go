package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
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

func NewRouter() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/offer", handleOffer).Methods("POST")
	router.HandleFunc("/stop", handleStop).Methods("POST")
	router.HandleFunc("/", serveHome).Methods("GET")

	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./client/")))

	return router
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

	// session.StopAllProcesses(appSession)
	appSession.StopAllProcesses()
	w.WriteHeader(http.StatusOK)
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
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

	mediaEngine := webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Failed to register default codecs: %v", err)
	}

	appSession.PeerConnection, err = webrtc.NewAPI(
		webrtc.WithMediaEngine(&mediaEngine),
	).NewPeerConnection(webrtc.Configuration{
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

	<-pipelineReady

	log.Println("Starting synth engine...")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err = appSession.Synth.Start()
		if err != nil {
			log.Println("Error starting synth engine:", err)
			http.Error(w, "Failed to start synth engine: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Println("Synth engine started successfully.")
	}()

	wg.Wait()

	<-gatherComplete

	sc.SendPlaySynthMessage(appSession.Synth.GetPort())

	localDescription := appSession.PeerConnection.LocalDescription()
	encodedLocalDesc, err := json.Marshal(localDescription)
	if err != nil {
		log.Println("Error encoding local description:", err)
		http.Error(w, "Failed to encode local description: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encodedLocalDesc)
}
