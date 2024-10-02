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
	SDP        string             `json:"sdp"`
	Type       string             `json:"type"`
	ICEServers []webrtc.ICEServer `json:"iceServers"`
}

// HandleOffer handles the incoming WebRTC offer
func HandleOffer(w http.ResponseWriter, r *http.Request) {
	log.Println("Received offer")
	offer, iceServers, err := processOffer(r)
	if err != nil {
		log.Printf("Error processing offer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process offer: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("Creating peer connection")
	peerConnection, err := createPeerConnection(iceServers)
	if err != nil {
		log.Printf("Error creating peer connection: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create peer connection: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Peer connection created with config: %+v", peerConnection.GetConfiguration())

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

	log.Println("Setting remote description")
	err = setRemoteDescription(peerConnection, *offer)
	if err != nil {
		log.Printf("Error setting remote description: %v", err)
		http.Error(w, fmt.Sprintf("Failed to set remote description: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("Creating answer")
	log.Printf("Creating answer with transceivers: %+v", peerConnection.GetTransceivers())
	answer, err := createAnswer(peerConnection)
	if err != nil {
		log.Printf("Error creating answer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create answer: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Answer SDP: %s", answer.SDP)

	log.Println("Finalizing connection setup")
	if err := finalizeConnectionSetup(appSession, audioTrack, answer); err != nil {
		log.Printf("Error finalizing connection setup: %v", err)
		http.Error(w, fmt.Sprintf("Failed to finalize connection setup: %v", err), http.StatusInternalServerError)
		return
	}

	appSession.Synth.SendPlayMessage()

	log.Println("Sending answer to client")
	sendAnswer(w, peerConnection.LocalDescription())
}

func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
	gatherComplete := webrtc.GatheringCompletePromise(appSession.PeerConnection)

	log.Println("Setting local description")
	if err := appSession.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Println("Error setting local description:", err)
		return fmt.Errorf("failed to set local description: %v", err)
	}

	log.Println("Starting media pipeline")
	if err := startMediaPipeline(appSession, audioTrack); err != nil {
		return err
	}

	log.Println("Starting synth engine")
	if err := startSynthEngine(appSession); err != nil {
		return err
	}

	log.Println("Waiting for ICE gathering to complete")
	<-gatherComplete
	log.Println("ICE gathering complete")
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

func processOffer(r *http.Request) (*webrtc.SessionDescription, []webrtc.ICEServer, error) {
	var browserOffer BrowserOffer

	log.Println("Decoding offer")
	err := json.NewDecoder(r.Body).Decode(&browserOffer)
	if err != nil {
		return nil, nil, err
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)
	log.Printf("Received offer: %v", offer.SDP)

	log.Printf("Offer SDP: %s", offer.SDP)

	return &offer, browserOffer.ICEServers, nil
}

func setSessionToConnection(w http.ResponseWriter, r *http.Request, peerConnection *webrtc.PeerConnection) (*session.AppSession, error) {
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		return nil, err
	}
	appSession.PeerConnection = peerConnection
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", state.String())
		log.Printf("Signaling State: %s\n", appSession.PeerConnection.SignalingState().String())
		log.Printf("Connection State: %s\n", appSession.PeerConnection.ConnectionState().String())

		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateDisconnected {
			log.Println("Connection failed or disconnected, initiating cleanup...")
			log.Printf("Last known ICE Candidates: %v\n", appSession.PeerConnection.GetStats())
			cleanUpSession(appSession)
		}
	})

	appSession.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("New ICE candidate: %s\n", candidate.String())
		} else {
			log.Println("All ICE candidates have been sent")
		}
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
func createPeerConnection(iceServers []webrtc.ICEServer) (*webrtc.PeerConnection, error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetEphemeralUDPPortRange(10000, 10100)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.relay.metered.ca:80"},
			},
			{
				URLs: []string{
					"turn:global.relay.metered.ca:80",
					"turn:global.relay.metered.ca:80?transport=tcp",
					"turn:global.relay.metered.ca:443",
					"turns:global.relay.metered.ca:443?transport=tcp",
				},
				Username:   "b6be1a94a4dbaa7c04a65bc9",
				Credential: "FLXvDM76W65uQiLc",
			},
		},
	}

	return api.NewPeerConnection(config)
}

// setRemoteDescription sets the offer as the remote description for the peer connection
func setRemoteDescription(pc *webrtc.PeerConnection, offer webrtc.SessionDescription) error {
	log.Printf("Setting remote description: %v", offer.SDP)
	sdp := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}
	return pc.SetRemoteDescription(sdp)
}

// createAnswer generates an SDP answer after setting the remote description
func createAnswer(pc *webrtc.PeerConnection) (webrtc.SessionDescription, error) {
	log.Println("Creating answer")
	return pc.CreateAnswer(nil)
}

// sendAnswer sends the generated answer as a response to the client
func sendAnswer(w http.ResponseWriter, answer *webrtc.SessionDescription) {
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		http.Error(w, "Failed to encode answer", http.StatusInternalServerError)
		return
	}
	log.Printf("Sending answer: %v", string(answerJSON))
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
	err = cleanUpSession(appSession)
	if err != nil {
		http.Error(w, "Error closing peer connection", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func cleanUpSession(appSession *session.AppSession) error {
	// Close the peer connection
	err := closePeerConnection(appSession.PeerConnection)
	if err != nil {
		log.Printf("Error closing peer connection: %v", err)
		return err
	}

	appSession.StopAllProcesses()
	return nil
}

// closePeerConnection gracefully closes the given peer connection
func closePeerConnection(pc *webrtc.PeerConnection) error {
	if pc == nil {
		return nil // If the peer connection is already nil, no need to close
	}
	log.Println("Closing peer connection")
	return pc.Close()
}
