package webrtc

import (
	"bytes"
	"encoding/base64"
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
	gst "github.com/po-studio/server/internal/gstreamer-src"
	"github.com/po-studio/server/internal/signal"
	"github.com/po-studio/server/session"
	"github.com/po-studio/server/synth"
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
	logWithTime("[OFFER] Received offer request from session: %s", sessionID)
	logWithTime("[OFFER] Request headers: %v", r.Header)

	// Read and log the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[OFFER][ERROR] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logWithTime("[OFFER] Raw request body: %s", string(body))

	// Restore the body for further processing
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	offer, iceServers, err := processOffer(r)
	if err != nil {
		log.Printf("[OFFER][ERROR] Error processing offer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process offer: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[OFFER] Processed offer details: Type=%s, ICEServers=%d", offer.Type, len(iceServers))
	logWithTime("[OFFER] SDP Preview: %.100s...", offer.SDP)

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

// why we need ice timeouts:
// - shorter timeouts for faster connection establishment
// - quick failure detection for better user experience
// - balanced between reliability and speed
const (
	iceGatheringTimeout = 15 * time.Second
	iceConnectTimeout   = 20 * time.Second
)

func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
	// Start media pipeline and synth engine in parallel with ICE gathering
	errChan := make(chan error, 2)

	// Start media pipeline async
	go func() {
		log.Println("Starting media pipeline")
		if err := startMediaPipeline(appSession, audioTrack); err != nil {
			errChan <- fmt.Errorf("media pipeline error: %v", err)
			return
		}
		errChan <- nil
	}()

	// Start synth engine async
	go func() {
		log.Println("Starting synth engine")
		if err := startSynthEngine(appSession); err != nil {
			errChan <- fmt.Errorf("synth engine error: %v", err)
			return
		}
		errChan <- nil
	}()

	// Set local description (this needs to happen before ICE gathering)
	log.Println("Setting local description")
	if err := appSession.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Println("Error setting local description:", err)
		return fmt.Errorf("failed to set local description: %v", err)
	}

	// Wait for ICE gathering with early success detection
	gatherComplete := webrtc.GatheringCompletePromise(appSession.PeerConnection)
	log.Println("Waiting for ICE gathering to complete (with timeout)")

	// Track successful candidate pairs
	var successfulPairs int
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[ICE] Connection state changed to %s for session %s", state.String(), appSession.Id)
		if state == webrtc.ICEConnectionStateChecking {
			stats := appSession.PeerConnection.GetStats()
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidatePairStats); ok && s.State == "succeeded" {
					successfulPairs++
					if successfulPairs == 1 {
						log.Printf("[ICE] Found first successful candidate pair, proceeding with connection")
					}
					log.Printf("[ICE] Found successful candidate pair: local=%s, remote=%s", s.LocalCandidateID, s.RemoteCandidateID)
				}
			}
		}
	})

	// Wait for pipeline and synth engine initialization
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}

	// Send play message immediately after synth is ready
	appSession.Synth.SendPlayMessage()

	select {
	case <-gatherComplete:
		log.Printf("[ICE] Gathering completed successfully")
		return nil
	case <-time.After(iceGatheringTimeout):
		stats := appSession.PeerConnection.GetStats()
		var candidateCount int
		for _, stat := range stats {
			if s, ok := stat.(*webrtc.ICECandidatePairStats); ok && s.State == "succeeded" {
				candidateCount++
			}
		}
		if candidateCount > 0 {
			log.Printf("[ICE] Gathering partial completion with %d valid candidates", candidateCount)
			return nil
		}
		log.Printf("[ICE] Gathering timed out after %v with no valid candidates", iceGatheringTimeout)
		return fmt.Errorf("ice gathering timeout with no valid candidates")
	}
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
	// Initialize monitoring before starting synth
	MonitorAudioPipeline(appSession)

	// Create error channel for synth startup
	errChan := make(chan error, 1)

	// Start synth in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(errChan)

		// Ensure synth is initialized
		if appSession.Synth == nil {
			appSession.Synth = synth.NewSuperColliderSynth(appSession.Id)
			appSession.Synth.SetOnClientName(func(clientName string) {
				appSession.JackClientName = clientName
			})
		}

		if err := appSession.Synth.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start synth engine: %v", err)
			return
		}
		log.Println("Synth engine started successfully.")
	}()

	wg.Wait()

	// Check for startup errors
	if err := <-errChan; err != nil {
		return err
	}

	return nil
}

func MonitorAudioPipeline(appSession *session.AppSession) {
	appSession.MonitorDone = make(chan struct{})

	// Monitor JACK connections with faster initial checks
	go func() {
		// Start with fast checks for the first few seconds
		fastTicker := time.NewTicker(3000 * time.Millisecond)
		defer fastTicker.Stop()

		// After 5 seconds, switch to slower checks
		time.AfterFunc(5*time.Second, func() {
			fastTicker.Stop()
		})

		slowTicker := time.NewTicker(5 * time.Second)
		defer slowTicker.Stop()

		for {
			select {
			case <-appSession.MonitorDone:
				return
			case <-fastTicker.C:
				checkJACKConnections(appSession)
			case <-slowTicker.C:
				checkJACKConnections(appSession)
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

func checkJACKConnections(appSession *session.AppSession) {
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[%s] Error monitoring JACK: %v", appSession.Id, err)
		return
	}
	log.Printf("[%s] JACK Connections:\n%s", appSession.Id, string(output))
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
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[ICE] Connection state changed to %s for session %s", state.String(), appSession.Id)

		if appSession.PeerConnection != nil {
			log.Printf("Signaling State: %s\n", appSession.PeerConnection.SignalingState().String())
			log.Printf("Connection State: %s\n", appSession.PeerConnection.ConnectionState().String())
		}

		switch state {
		case webrtc.ICEConnectionStateFailed:
			// Complete failure - clean up everything
			log.Printf("[ICE][ERROR] Connection failed for session %s, initiating cleanup...", appSession.Id)
			if appSession.PeerConnection != nil {
				stats := appSession.PeerConnection.GetStats()
				log.Printf("[ICE] Last known ICE stats for session %s: %+v", appSession.Id, stats)
			}
			cleanUpSession(appSession)

		case webrtc.ICEConnectionStateDisconnected:
			// Temporary disconnection - wait for potential reconnect
			log.Printf("[ICE] Connection disconnected for session %s, waiting for reconnection...", appSession.Id)
			// Start a timer to cleanup only if disconnection persists
			time.AfterFunc(30*time.Second, func() {
				if appSession.PeerConnection != nil &&
					appSession.PeerConnection.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected {
					log.Printf("[ICE] Disconnection timeout for session %s, cleaning up", appSession.Id)
					cleanUpSession(appSession)
				}
			})
		}
	})

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

// why we need ice configuration validation:
// - ensures STUN servers are properly configured
// - validates URL format
// - verifies required parameters are present
// - checks for TURN server availability
func verifyICEConfiguration(iceServers []webrtc.ICEServer) error {
	if len(iceServers) == 0 {
		return fmt.Errorf("no ICE servers provided")
	}

	hasSTUN := false
	hasTURN := false

	for i, server := range iceServers {
		log.Printf("[ICE] Server %d configuration:", i)
		log.Printf("  - URLs: %v", server.URLs)
		log.Printf("  - Username: %v", server.Username != "")
		log.Printf("  - Credential: %v", server.Credential != nil)

		for _, url := range server.URLs {
			if strings.HasPrefix(url, "stun:") {
				hasSTUN = true
				log.Printf("[ICE] Found valid STUN URL: %s", url)
			} else if strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:") {
				hasTURN = true
				log.Printf("[ICE] Found valid TURN URL: %s", url)
				if server.Username == "" || server.Credential == nil {
					log.Printf("[ICE][WARN] TURN server missing credentials")
				}
			}
		}
	}

	if !hasSTUN {
		return fmt.Errorf("no valid STUN URLs found in ICE server configuration")
	}

	if !hasTURN {
		log.Printf("[ICE][WARN] No TURN servers configured. This may cause connectivity issues for clients behind symmetric NATs")
	}

	return nil
}

// createPeerConnection initializes a new WebRTC peer connection
func createPeerConnection(iceServers []webrtc.ICEServer) (*webrtc.PeerConnection, error) {
	s := webrtc.SettingEngine{}
	s.SetEphemeralUDPPortRange(10000, 10100)

	// ice timeouts for STUN:
	// - disconnectedTimeout: time to wait before considering a connection lost
	// - failedTimeout: time to wait before giving up on reconnection
	// - keepAliveInterval: how often to send keepalive packets
	s.SetICETimeouts(
		5*time.Second,        // disconnectedTimeout (reduced from 10s)
		7*time.Second,        // failedTimeout (reduced from 15s)
		250*time.Millisecond, // keepAliveInterval (reduced from 500ms)
	)

	// candidate gathering timeouts:
	// - reduced timeouts for faster connection establishment
	// - still allowing enough time for STUN candidates
	s.SetHostAcceptanceMinWait(250 * time.Millisecond)   // reduced from 500ms
	s.SetSrflxAcceptanceMinWait(1000 * time.Millisecond) // reduced from 2000ms
	s.SetPrflxAcceptanceMinWait(250 * time.Millisecond)  // reduced from 500ms

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("failed to register codecs: %v", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
	)

	config := webrtc.Configuration{
		ICEServers:           iceServers,
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		ICECandidatePoolSize: 2,
		RTCPMuxPolicy:        webrtc.RTCPMuxPolicyRequire,
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Add detailed connection state monitoring
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Connection state changed to: %s", state.String())
		if state == webrtc.PeerConnectionStateConnecting {
			// Log ICE candidates when connecting
			stats := pc.GetStats()
			for _, stat := range stats {
				if candidatePair, ok := stat.(*webrtc.ICECandidatePairStats); ok {
					log.Printf("[ICE] Candidate pair: state=%s nominated=%v",
						candidatePair.State, candidatePair.Nominated)
				}
			}
		}
	})

	// Monitor ICE gathering state
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		log.Printf("[ICE] Gathering state changed to: %s", state.String())
		if state == webrtc.ICEGathererStateComplete {
			stats := pc.GetStats()
			var candidateCount int
			for _, stat := range stats {
				if candidatePair, ok := stat.(*webrtc.ICECandidatePairStats); ok {
					if candidatePair.State == "succeeded" {
						candidateCount++
						log.Printf("[ICE] Successful candidate pair: local=%s remote=%s",
							candidatePair.LocalCandidateID, candidatePair.RemoteCandidateID)
					}
				}
			}
			log.Printf("[ICE] Found %d successful candidate pairs", candidateCount)
		}
	})

	// why we need connection state monitoring:
	// - detect browser window closes
	// - ensure cleanup on unexpected disconnects
	// - prevent orphaned jack connections
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Connection state changed to: %s", state.String())

		// Clean up when connection is closed or failed
		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {

			// Get session from connection
			sessionID := ""
			for _, stats := range pc.GetStats() {
				if transportStats, ok := stats.(*webrtc.TransportStats); ok {
					sessionID = transportStats.ID
					break
				}
			}

			if sessionID != "" {
				if appSession, exists := session.GetSession(sessionID); exists {
					log.Printf("[CLEANUP] Connection state %s triggered cleanup for session %s", state, sessionID)
					appSession.StopAllProcesses()
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

// why we need session-specific cleanup:
// - multiple clients may have active sessions
// - stopping one synth shouldn't affect others
// - must preserve other sessions' resources
func HandleStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	log.Printf("[STOP] Received stop request for session: %s", sessionID)

	// Get the specific session to stop
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		log.Printf("[STOP][ERROR] Session not found: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Use StopAllProcesses for thorough cleanup of this session only
	appSession.StopAllProcesses()

	log.Printf("[STOP] Successfully stopped session: %s", sessionID)
	w.WriteHeader(http.StatusOK)
}

func cleanUpSession(appSession *session.AppSession) error {
	log.Printf("[CLEANUP] Starting cleanup for session %s", appSession.Id)

	// First stop monitoring to prevent any new operations
	if appSession.MonitorDone != nil {
		close(appSession.MonitorDone)
		appSession.MonitorDone = nil
	}

	// Stop the synth engine first as it depends on other components
	if appSession.Synth != nil {
		if err := appSession.Synth.Stop(); err != nil {
			log.Printf("[CLEANUP][ERROR] Failed to stop synth: %v", err)
		}
		appSession.Synth = nil
	}

	// Stop the GStreamer pipeline
	if appSession.GStreamerPipeline != nil {
		appSession.GStreamerPipeline.Stop()
		appSession.GStreamerPipeline = nil
	}

	// Close the peer connection last
	if appSession.PeerConnection != nil {
		// Get final stats before closing
		stats := appSession.PeerConnection.GetStats()
		log.Printf("[CLEANUP] Final WebRTC stats: %+v", stats)

		// Close all transceivers
		for _, t := range appSession.PeerConnection.GetTransceivers() {
			if err := t.Stop(); err != nil {
				log.Printf("[CLEANUP][ERROR] Failed to stop transceiver: %v", err)
			}
		}

		// Remove all tracks
		for _, sender := range appSession.PeerConnection.GetSenders() {
			if err := appSession.PeerConnection.RemoveTrack(sender); err != nil {
				log.Printf("[CLEANUP][ERROR] Failed to remove track: %v", err)
			}
		}

		// Close the connection
		if err := appSession.PeerConnection.Close(); err != nil {
			log.Printf("[CLEANUP][ERROR] Failed to close peer connection: %v", err)
		}
		appSession.PeerConnection = nil
	}

	log.Printf("[CLEANUP] Completed cleanup for session %s", appSession.Id)
	return nil
}

// why we need robust ice candidate handling:
// - ensures reliable peer connections
// - handles network changes gracefully
// - improves connection stability
func HandleICECandidate(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	logWithTime("[ICE] Received candidate for session: %s", sessionID)

	// Get session and validate it exists
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		log.Printf("[ICE][ERROR] Session not found: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Ensure peer connection exists
	if appSession.PeerConnection == nil {
		log.Printf("[ICE][ERROR] No peer connection for session %s", sessionID)
		http.Error(w, "No peer connection", http.StatusBadRequest)
		return
	}

	// Read and log the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ICE][ERROR] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logWithTime("[ICE] Raw request body: %s", string(body))

	// First unmarshal the outer wrapper
	var wrapper struct {
		Candidate string `json:"candidate"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&wrapper); err != nil {
		log.Printf("[ICE][ERROR] Failed to decode outer wrapper: %v", err)
		log.Printf("[ICE][DEBUG] Attempted to decode body: %s", string(body))
		http.Error(w, "Invalid candidate format", http.StatusBadRequest)
		return
	}
	log.Printf("[ICE][DEBUG] Decoded wrapper candidate: %s", wrapper.Candidate)

	// Decode the base64 string
	candidateJSON, err := base64.StdEncoding.DecodeString(wrapper.Candidate)
	if err != nil {
		log.Printf("[ICE][ERROR] Failed to decode base64: %v", err)
		log.Printf("[ICE][DEBUG] Attempted to decode base64: %s", wrapper.Candidate)
		http.Error(w, "Invalid base64 encoding", http.StatusBadRequest)
		return
	}
	log.Printf("[ICE][DEBUG] Decoded base64 JSON: %s", string(candidateJSON))

	// Try to parse as a direct candidate string first
	var candidateObj struct {
		Candidate        string `json:"candidate"`
		SDPMid           string `json:"sdpMid"`
		SDPMLineIndex    uint16 `json:"sdpMLineIndex"`
		UsernameFragment string `json:"usernameFragment"`
	}
	if err := json.Unmarshal(candidateJSON, &candidateObj); err != nil {
		log.Printf("[ICE][ERROR] Failed to decode candidate JSON: %v", err)
		log.Printf("[ICE][DEBUG] Attempted to decode JSON: %s", string(candidateJSON))
		http.Error(w, "Invalid candidate format", http.StatusBadRequest)
		return
	}
	log.Printf("[ICE][DEBUG] Parsed candidate: %+v", candidateObj)

	// Create and log ICE candidate init
	candidate := webrtc.ICECandidateInit{
		Candidate:        candidateObj.Candidate,
		SDPMid:           &candidateObj.SDPMid,
		SDPMLineIndex:    &candidateObj.SDPMLineIndex,
		UsernameFragment: &candidateObj.UsernameFragment,
	}
	log.Printf("[ICE][DEBUG] Created ICE candidate init: %+v", candidate)

	// Add the candidate and log result
	if err := appSession.PeerConnection.AddICECandidate(candidate); err != nil {
		log.Printf("[ICE][ERROR] Failed to add candidate: %v", err)
		log.Printf("[ICE][DEBUG] Failed candidate details: %+v", candidate)
		http.Error(w, fmt.Sprintf("Failed to add candidate: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("[ICE][SUCCESS] Added ICE candidate for session %s", sessionID)

	w.WriteHeader(http.StatusOK)
}

func getTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func logWithTime(format string, v ...interface{}) {
	timestamp := getTimestamp()
	log.Printf("[%s] %s", timestamp, fmt.Sprintf(format, v...))
}
