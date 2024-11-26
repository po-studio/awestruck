package webrtc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

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

type ICECandidateRequest struct {
	Candidate struct {
		Candidate        string `json:"candidate"`
		SDPMid           string `json:"sdpMid"`
		SDPMLineIndex    uint16 `json:"sdpMLineIndex"`
		UsernameFragment string `json:"usernameFragment"`
	} `json:"candidate"`
}

// HandleOffer handles the incoming WebRTC offer
func HandleOffer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	log.Printf("[OFFER] Received offer request from session: %s", sessionID)
	log.Printf("[OFFER] Request headers: %+v", r.Header)

	// Read and log the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[OFFER][ERROR] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	log.Printf("[OFFER] Raw request body: %s", string(body))

	// Restore the body for further processing
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	offer, iceServers, err := processOffer(r)
	if err != nil {
		log.Printf("[OFFER][ERROR] Error processing offer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process offer: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[OFFER] Processed offer details: Type=%s, ICEServers=%d", offer.Type, len(iceServers))
	log.Printf("[OFFER] SDP Preview: %.100s...", offer.SDP)

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

// func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
// 	gatherComplete := webrtc.GatheringCompletePromise(appSession.PeerConnection)

// 	log.Println("Setting local description")
// 	if err := appSession.PeerConnection.SetLocalDescription(answer); err != nil {
// 		log.Println("Error setting local description:", err)
// 		return fmt.Errorf("failed to set local description: %v", err)
// 	}

// 	log.Println("Starting media pipeline")
// 	if err := startMediaPipeline(appSession, audioTrack); err != nil {
// 		return err
// 	}

// 	log.Println("Starting synth engine")
// 	if err := startSynthEngine(appSession); err != nil {
// 		return err
// 	}

// 	log.Println("Waiting for ICE gathering to complete")
// 	<-gatherComplete
// 	log.Println("ICE gathering complete")
// 	return nil
// }

func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
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

	log.Println("[OFFER] Decoding offer JSON")
	err := json.NewDecoder(r.Body).Decode(&browserOffer)
	if err != nil {
		log.Printf("[OFFER][ERROR] JSON decode failed: %v", err)
		return nil, nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	log.Printf("[OFFER] Decoded offer: Type=%s, SDPLength=%d",
		browserOffer.Type,
		len(browserOffer.SDP))

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	log.Printf("[OFFER] Decoded SDP details:")
	log.Printf("[OFFER] - Type: %s", offer.Type)
	log.Printf("[OFFER] - SDP Length: %d", len(offer.SDP))
	log.Printf("[OFFER] - SDP Preview: %.100s...", offer.SDP)
	log.Printf("[OFFER] - ICE Servers count: %d", len(browserOffer.ICEServers))

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
			log.Printf("[ICE][ERROR] Connection %s for session %s, initiating cleanup...",
				state.String(), appSession.Id)
			stats := appSession.PeerConnection.GetStats()
			log.Printf("[ICE] Last known ICE stats for session %s: %+v",
				appSession.Id, stats)
			cleanUpSession(appSession)
		}
	})

	appSession.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("[ICE] New candidate for session %s: type=%s protocol=%s address=%s port=%d priority=%d",
				appSession.Id,
				candidate.Component,
				candidate.Protocol,
				candidate.Address,
				candidate.Port,
				candidate.Priority)
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
	settingEngine.SetICETimeouts(
		5*time.Second,  // Disconnected timeout
		10*time.Second, // Failed timeout
		5*time.Second,  // Keepalive interval
	)
	settingEngine.SetEphemeralUDPPortRange(10000, 10010)
	settingEngine.SetLite(true)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	config := webrtc.Configuration{
		ICEServers:         iceServers,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
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
	log.Println("[ANSWER] Creating answer")
	log.Printf("[ANSWER] Current connection state: %s", pc.ConnectionState().String())
	log.Printf("[ANSWER] Current signaling state: %s", pc.SignalingState().String())

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("[ANSWER][ERROR] Failed to create answer: %v", err)
		return webrtc.SessionDescription{}, err
	}

	log.Printf("[ANSWER] Created answer: Type=%s, SDPLength=%d",
		answer.Type,
		len(answer.SDP))
	log.Printf("[ANSWER] SDP Preview: %.100s...", answer.SDP)

	return answer, nil
}

// sendAnswer sends the generated answer as a response to the client
func sendAnswer(w http.ResponseWriter, answer *webrtc.SessionDescription) {
	log.Println("[ANSWER] Preparing to send answer")

	answerJSON, err := json.Marshal(answer)
	if err != nil {
		log.Printf("[ANSWER][ERROR] Failed to encode answer: %v", err)
		http.Error(w, "Failed to encode answer", http.StatusInternalServerError)
		return
	}

	log.Printf("[ANSWER] Sending JSON response: %s", string(answerJSON))
	w.Header().Set("Content-Type", "application/json")
	w.Write(answerJSON)
	log.Println("[ANSWER] Answer sent successfully")
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

func HandleICECandidate(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	log.Printf("[ICE] Handling candidate for session: %s from IP: %s",
		sessionID, r.RemoteAddr)

	var candidateReq ICECandidateRequest
	if err := json.NewDecoder(r.Body).Decode(&candidateReq); err != nil {
		log.Printf("[ICE][ERROR] Failed to decode candidate for session %s: %v",
			sessionID, err)
		http.Error(w, "Failed to decode ICE candidate", http.StatusBadRequest)
		return
	}

	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		log.Printf("[ICE][ERROR] Failed to get/create session %s: %v",
			sessionID, err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if appSession.PeerConnection == nil {
		log.Printf("[ICE][ERROR] No peer connection for session %s", sessionID)
		http.Error(w, "No peer connection established", http.StatusBadRequest)
		return
	}

	candidate := webrtc.ICECandidateInit{
		Candidate:        candidateReq.Candidate.Candidate,
		SDPMid:           &candidateReq.Candidate.SDPMid,
		SDPMLineIndex:    &candidateReq.Candidate.SDPMLineIndex,
		UsernameFragment: &candidateReq.Candidate.UsernameFragment,
	}

	if err := appSession.PeerConnection.AddICECandidate(candidate); err != nil {
		log.Printf("[ICE][ERROR] Failed to add candidate for session %s: %v",
			sessionID, err)
		http.Error(w, "Failed to add ICE candidate", http.StatusInternalServerError)
		return
	}

	log.Printf("[ICE] Successfully added candidate for session: %s", sessionID)
	w.WriteHeader(http.StatusOK)
}
