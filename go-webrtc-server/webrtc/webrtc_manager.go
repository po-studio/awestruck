package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/pion/webrtc/v3"
	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/internal/signal"
	"github.com/po-studio/go-webrtc-server/session"
)

// BrowserOffer represents the SDP offer from the browser
type BrowserOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

// HandleOffer handles the incoming WebRTC offer
func HandleOffer(w http.ResponseWriter, r *http.Request) {
	offer, err := processOffer(r)
	if err != nil {
		log.Printf("Error processing request offer: %v", err)
		http.Error(w, "Failed to process peer request", http.StatusInternalServerError)
		return
	}

	peerConnection, err := createPeerConnection()
	if err != nil {
		log.Printf("Error creating peer connection: %v", err)
		http.Error(w, "Failed to create peer connection", http.StatusInternalServerError)
		return
	}

	appSession, err := setSessionToConnection(w, r, peerConnection)
	if err != nil {
		http.Error(w, "Failed to set session to peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audioTrack, err := prepareMedia(*appSession)
	if err != nil {
		http.Error(w, "Failed to create audio track or add to the peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = setRemoteDescription(appSession.PeerConnection, *offer)
	if err != nil {
		http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
		return
	}

	answer, err := createAnswer(appSession.PeerConnection)
	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		return
	}

	if err := finalizeConnectionSetup(appSession, audioTrack, answer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	appSession.Synth.SendPlayMessage()

	sendAnswer(w, appSession.PeerConnection.LocalDescription())
}

func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
	gatherComplete := webrtc.GatheringCompletePromise(appSession.PeerConnection)

	if err := appSession.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Println("Error setting local description:", err)
		return fmt.Errorf("failed to set local description: %v", err)
	}

	if err := startMediaPipeline(appSession, audioTrack); err != nil {
		return err
	}

	if err := startSynthEngine(appSession); err != nil {
		return err
	}

	<-gatherComplete
	return nil
}

func startMediaPipeline(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample) error {
	pipelineReady := make(chan struct{})

	go func() {
		log.Println("Creating pipeline...")
		appSession.GStreamerPipeline = gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, *appSession.AudioSrc)
		appSession.GStreamerPipeline.Start()
		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	<-pipelineReady
	return nil
}

func startSynthEngine(appSession *session.AppSession) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := appSession.Synth.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start synth engine: %v", err)
			return
		}
		log.Println("Synth engine started successfully.")
	}()

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return err
	}

	appSession.Synth.SendPlayMessage()
	return nil
}

func processOffer(r *http.Request) (*webrtc.SessionDescription, error) {
	var browserOffer BrowserOffer

	err := json.NewDecoder(r.Body).Decode(&browserOffer)
	if err != nil {
		return nil, err
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	return &offer, nil
}

func setSessionToConnection(w http.ResponseWriter, r *http.Request, peerConnection *webrtc.PeerConnection) (*session.AppSession, error) {
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		return nil, err
	}
	appSession.PeerConnection = peerConnection
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", state.String())
	})

	return appSession, nil
}

func prepareMedia(appSession session.AppSession) (*webrtc.TrackLocalStaticSample, error) {
	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion1")
	if err != nil {
		log.Printf("Failed to create audio track: %v\n", err)
		return nil, err
	}

	_, err = appSession.PeerConnection.AddTrack(audioTrack)
	if err != nil {
		log.Printf("Failed to add audio track to the peer connection: %v\n", err)
		return nil, err
	}

	return audioTrack, nil
}

// createPeerConnection initializes a new WebRTC peer connection
func createPeerConnection() (*webrtc.PeerConnection, error) {
	mediaEngine := webrtc.MediaEngine{}
	err := mediaEngine.RegisterDefaultCodecs()
	if err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}
	return peerConnection, nil
}

// setRemoteDescription sets the offer as the remote description for the peer connection
func setRemoteDescription(pc *webrtc.PeerConnection, offer webrtc.SessionDescription) error {
	sdp := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}
	return pc.SetRemoteDescription(sdp)
}

// createAnswer generates an SDP answer after setting the remote description
func createAnswer(pc *webrtc.PeerConnection) (webrtc.SessionDescription, error) {
	return pc.CreateAnswer(nil)
}

// sendAnswer sends the generated answer as a response to the client
func sendAnswer(w http.ResponseWriter, answer *webrtc.SessionDescription) {
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		http.Error(w, "Failed to encode answer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(answerJSON)
}

// HandleStop processes the stop request for a WebRTC session
func HandleStop(w http.ResponseWriter, r *http.Request) {
	log.Println("Stop signal received, cleaning up...")

	// Retrieve the session from the request
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Close the peer connection
	err = closePeerConnection(appSession.PeerConnection)
	if err != nil {
		log.Printf("Error closing peer connection: %v", err)
		http.Error(w, "Error closing peer connection", http.StatusInternalServerError)
		return
	}

	appSession.StopAllProcesses()

	w.WriteHeader(http.StatusOK)
}

// ClosePeerConnection gracefully closes the given peer connection
func closePeerConnection(pc *webrtc.PeerConnection) error {
	if pc == nil {
		return nil // If the peer connection is already nil, no need to close
	}
	return pc.Close()
}
