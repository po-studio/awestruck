package webrtc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	awestruckConfig "github.com/po-studio/go-webrtc-server/config"
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

	if err := verifyICEConfiguration(iceServers); err != nil {
		log.Printf("[ERROR] Invalid ICE configuration: %v", err)
		http.Error(w, fmt.Sprintf("Invalid ICE configuration: %v", err), http.StatusBadRequest)
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
	if err := finalizeConnectionSetup(appSession, audioTrack, *answer); err != nil {
		log.Printf("Error finalizing connection setup: %v", err)
		http.Error(w, fmt.Sprintf("Failed to finalize connection setup: %v", err), http.StatusInternalServerError)
		return
	}

	// appSession.Synth.SendPlayMessage()

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

	// Add timeout for ICE gathering
	log.Println("Waiting for ICE gathering to complete (with timeout)")
	select {
	case <-gatherComplete:
		log.Println("ICE gathering completed successfully")
	case <-time.After(5 * time.Second):
		log.Println("ICE gathering timed out after 5 seconds, proceeding with available candidates")
	}

	return nil
}

func startMediaPipeline(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample) error {
	pipelineReady := make(chan struct{})

	go func() {
		log.Println("Creating pipeline...")
		log.Printf("Using pipeline config: %s", *appSession.AudioSrc)
		appSession.GStreamerPipeline = gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, *appSession.AudioSrc)
		appSession.GStreamerPipeline.Start()
		log.Println("Pipeline created and started")
		close(pipelineReady)
	}()

	<-pipelineReady
	return nil
}

func startSynthEngine(appSession *session.AppSession) error {
	MonitorAudioPipeline(appSession)
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

func MonitorAudioPipeline(appSession *session.AppSession) {
	appSession.MonitorDone = make(chan struct{})

	// Monitor JACK connections
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-appSession.MonitorDone:
				return
			case <-ticker.C:
				cmd := exec.Command("jack_lsp", "-c")
				output, err := cmd.CombinedOutput()
				if err != nil {
					log.Printf("[%s] Error monitoring JACK: %v", appSession.Id, err)
					continue
				}
				log.Printf("[%s] JACK Connections:\n%s", appSession.Id, string(output))
			}
		}
	}()

	// Monitor WebRTC stats
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if appSession.PeerConnection != nil {
				stats := appSession.PeerConnection.GetStats()
				for _, stat := range stats {
					switch s := stat.(type) {
					case *webrtc.OutboundRTPStreamStats:
						log.Printf("[%s] Outbound RTP: packets=%d bytes=%d",
							appSession.Id, s.PacketsSent, s.BytesSent)
					case *webrtc.InboundRTPStreamStats:
						log.Printf("[%s] Inbound RTP: packets=%d bytes=%d jitter=%v",
							appSession.Id, s.PacketsReceived, s.BytesReceived, s.Jitter)
					}
				}
			}
		}
	}()
}

func processOffer(r *http.Request) (*webrtc.SessionDescription, []webrtc.ICEServer, error) {
	var browserOffer BrowserOffer

	log.Println("[OFFER] Decoding offer JSON")
	err := json.NewDecoder(r.Body).Decode(&browserOffer)
	if err != nil {
		log.Printf("[OFFER][ERROR] JSON decode failed: %v", err)
		return nil, nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	// Create a MediaEngine and populate it from the SDP
	mediaEngine := webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, nil, fmt.Errorf("failed to register default codecs: %v", err)
	}

	log.Printf("[OFFER] Media direction in offer: %v", offer.SDP)

	// Log detailed SDP analysis
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

	// Track connection state changes
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Connection state changed to %s for session %s", state.String(), appSession.Id)

		switch state {
		case webrtc.PeerConnectionStateFailed:
			log.Printf("[WebRTC][ERROR] Connection failed for session %s, initiating cleanup...", appSession.Id)
			cleanUpSession(appSession)
		case webrtc.PeerConnectionStateDisconnected:
			log.Printf("[WebRTC] Connection disconnected for session %s", appSession.Id)
			// Faster cleanup for disconnection
			time.AfterFunc(5*time.Second, func() {
				if appSession.PeerConnection != nil &&
					appSession.PeerConnection.ConnectionState() == webrtc.PeerConnectionStateDisconnected {
					log.Printf("[WebRTC] Disconnection timeout for session %s, cleaning up", appSession.Id)
					cleanUpSession(appSession)
				}
			})
		}
	})

	// Track ICE connection state changes
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[ICE] Connection state changed to %s for session %s", state.String(), appSession.Id)

		if appSession.PeerConnection != nil {
			log.Printf("Signaling State: %s\n", appSession.PeerConnection.SignalingState().String())
			log.Printf("Connection State: %s\n", appSession.PeerConnection.ConnectionState().String())
		}

		switch state {
		case webrtc.ICEConnectionStateFailed:
			log.Printf("[ICE][ERROR] Connection failed for session %s, initiating cleanup...", appSession.Id)
			if appSession.PeerConnection != nil {
				stats := appSession.PeerConnection.GetStats()
				log.Printf("[ICE] Last known ICE stats for session %s: %+v", appSession.Id, stats)
			}
			cleanUpSession(appSession)

		case webrtc.ICEConnectionStateDisconnected:
			log.Printf("[ICE] Connection disconnected for session %s, waiting for reconnection...", appSession.Id)
			// Faster cleanup for ICE disconnection
			time.AfterFunc(5*time.Second, func() {
				if appSession.PeerConnection != nil &&
					appSession.PeerConnection.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected {
					log.Printf("[ICE] Disconnection timeout for session %s, cleaning up", appSession.Id)
					cleanUpSession(appSession)
				}
			})
		}
	})

	// Track ICE candidates
	appSession.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("[ICE] New candidate for session %s: type=%d protocol=%s address=%s port=%d priority=%d",
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
	// Create the audio track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"pion1",
	)
	if err != nil {
		log.Printf("Failed to create audio track: %v\n", err)
		return nil, err
	}

	// Add the audio track to the peer connection
	_, err = appSession.PeerConnection.AddTrack(audioTrack)
	if err != nil {
		log.Printf("Failed to add audio track: %v\n", err)
		return nil, err
	}

	log.Printf("Added audio track with ID: %v\n", audioTrack.ID())
	return audioTrack, nil
}

// createPeerConnection initializes a new WebRTC peer connection
func createPeerConnection(iceServers []webrtc.ICEServer) (*webrtc.PeerConnection, error) {
	// Create a SettingEngine and configure timeouts
	s := webrtc.SettingEngine{}
	s.SetEphemeralUDPPortRange(10000, 10100)
	s.SetICETimeouts(
		2*time.Second, // disconnectedTimeout
		5*time.Second, // failedTimeout
		1*time.Second, // keepAliveInterval
	)

	// Create MediaEngine
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("failed to register codecs: %v", err)
	}

	// Create API with our settings
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
	)

	config := webrtc.Configuration{
		ICEServers:           iceServers,
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		ICECandidatePoolSize: 1,
	}

	// Only force TURN relay in production/staging environments
	if awestruckConfig.Get().Environment != "development" {
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Add logging for ICE connection states
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[ICE] Connection state changed: %s", state.String())
		if state == webrtc.ICEConnectionStateConnected {
			log.Printf("[ICE] Connection established successfully")
			// Log active candidate pair
			stats := pc.GetStats()
			for _, s := range stats {
				if candidatePair, ok := s.(*webrtc.ICECandidatePairStats); ok && candidatePair.State == "succeeded" {
					log.Printf("[ICE] Active candidate pair: %+v", candidatePair)
				}
			}
		}
	})

	return pc, nil
}

// setRemoteDescription sets the offer as the remote description for the peer connection
func setRemoteDescription(pc *webrtc.PeerConnection, offer webrtc.SessionDescription) error {
	log.Printf("Setting remote description: %v", offer.SDP)
	sdp := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}
	return pc.SetRemoteDescription(sdp)
}

// createAnswer generates an SDP answer after setting the remote description
func createAnswer(pc *webrtc.PeerConnection) (*webrtc.SessionDescription, error) {
	log.Printf("[ANSWER] Creating answer")
	log.Printf("[ANSWER] Current connection state: %s", pc.ConnectionState().String())
	log.Printf("[ANSWER] Current signaling state: %s", pc.SignalingState().String())

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("[ANSWER][ERROR] Failed to create answer: %v", err)
		return nil, err
	}

	log.Printf("[ANSWER] Created answer successfully")
	return &answer, nil
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
	if appSession == nil {
		return nil
	}

	// Ensure we don't try to clean up the same session multiple times
	if appSession.PeerConnection == nil {
		return nil
	}

	// Get final stats before cleanup
	if stats := appSession.PeerConnection.GetStats(); stats != nil {
		log.Printf("[Cleanup] Final WebRTC stats for session %s: %+v", appSession.Id, stats)
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
	log.Printf("[ICE] Received candidate for session: %s", sessionID)

	var candidateRequest ICECandidateRequest
	if err := json.NewDecoder(r.Body).Decode(&candidateRequest); err != nil {
		log.Printf("[ICE][ERROR] Failed to decode candidate: %v", err)
		http.Error(w, "Invalid ICE candidate", http.StatusBadRequest)
		return
	}

	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		log.Printf("[ICE][ERROR] Failed to get/create session %s: %v",
			sessionID, err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	candidate := webrtc.ICECandidateInit{
		Candidate:        candidateRequest.Candidate.Candidate,
		SDPMid:           &candidateRequest.Candidate.SDPMid,
		SDPMLineIndex:    &candidateRequest.Candidate.SDPMLineIndex,
		UsernameFragment: &candidateRequest.Candidate.UsernameFragment,
	}

	log.Printf("[ICE] Adding candidate for session %s: %+v", sessionID, candidate)
	if err := appSession.PeerConnection.AddICECandidate(candidate); err != nil {
		log.Printf("[ICE][ERROR] Failed to add ICE candidate: %v", err)
		http.Error(w, "Failed to add ICE candidate", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func verifyICEConfiguration(iceServers []webrtc.ICEServer) error {
	if len(iceServers) == 0 {
		return fmt.Errorf("no ICE servers provided")
	}

	for i, server := range iceServers {
		log.Printf("[ICE] Server %d configuration:", i)
		log.Printf("  - URLs: %v", server.URLs)
		log.Printf("  - Username length: %d", len(server.Username))
		log.Printf("  - Credential length: %d", len(server.Credential.(string)))

		// Verify TURN URLs are present when in relay-only mode
		hasTURN := false
		for _, url := range server.URLs {
			if strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:") {
				hasTURN = true
				break
			}
		}

		if !hasTURN {
			return fmt.Errorf("no TURN URLs found in ICE server configuration")
		}
	}

	return nil
}
