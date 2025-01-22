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
	"github.com/po-studio/server/config"
	gst "github.com/po-studio/server/internal/gstreamer-src"
	"github.com/po-studio/server/internal/signal"
	"github.com/po-studio/server/session"
	"github.com/po-studio/server/synth"
)

// BrowserOffer represents the SDP offer from the browser
type BrowserOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

type ICECandidateRequest struct {
	Candidate struct {
		Candidate        string `json:"candidate"`
		SDPMid           string `json:"sdpMid"`
		SDPMLineIndex    uint16 `json:"sdpMLineIndex"`
		UsernameFragment string `json:"usernameFragment"`
	} `json:"candidate"`
}

// why we need consistent ice credentials:
// - must match turn server config
// - ensures authentication works
// - meets webrtc security requirements
func getICECredentials() (string, string) {
	// use same username as turn server
	username := "dummy-username"
	password := "c07f1c92982b4621af1d1a63dda5539c"
	return username, password
}

// why we need flexible ice configuration:
// - allows both direct and relay connections
// - improves connection reliability
// - reduces latency when possible
func getICEServers() []webrtc.ICEServer {
	hostname := config.GetTurnServer()
	username, password := getICECredentials()

	return []webrtc.ICEServer{
		{
			URLs: []string{
				fmt.Sprintf("stun:%s:3478", hostname),
				fmt.Sprintf("turn:%s:3478", hostname),
			},
			Username:   username,
			Credential: password,
		},
	}
}

// why we need a config endpoint:
// - provides ice configuration to client
// - ensures consistent settings
// - enables environment-specific config
func HandleConfig(w http.ResponseWriter, r *http.Request) {
	config := webrtc.Configuration{
		ICEServers:           getICEServers(),
		ICETransportPolicy:   webrtc.ICETransportPolicyAll, // Allow both direct and relay
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:        webrtc.RTCPMuxPolicyRequire,
		ICECandidatePoolSize: 4, // Increased for better candidate gathering
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// HandleOffer handles the incoming WebRTC offer from the browser and sets up the peer connection.
// It processes the SDP offer, creates a peer connection, and sends back an SDP answer.
func HandleOffer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	logWithTime("[OFFER] Received offer request from session: %s", sessionID)
	logWithTime("[OFFER] Request headers: %v", r.Header)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logWithTime("[OFFER][ERROR] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logWithTime("[OFFER] Raw request body: %s", string(body))

	// Restore the body for further processing
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	offer, err := processOffer(r)
	if err != nil {
		logWithTime("[OFFER][ERROR] Error processing offer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to process offer: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[OFFER] Processed offer details: Type=%s", offer.Type)
	logWithTime("[OFFER] SDP Preview: %.100s...", offer.SDP)

	// Use server's ICE configuration
	iceServers := getICEServers()
	logWithTime("[WEBRTC] Creating peer connection with ICE servers: %+v", iceServers)

	peerConnection, err := createPeerConnection(iceServers, sessionID)
	if err != nil {
		logWithTime("[WEBRTC][ERROR] Error creating peer connection: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create peer connection: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[WEBRTC] Peer connection created with config: %+v", peerConnection.GetConfiguration())

	appSession, err := setSessionToConnection(w, r, peerConnection)
	if err != nil {
		logWithTime("[WEBRTC][ERROR] Failed to set session to peer connection: %v", err)
		http.Error(w, "Failed to set session to peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audioTrack, err := prepareMedia(*appSession)
	if err != nil {
		logWithTime("[MEDIA][ERROR] Failed to create audio track: %v", err)
		http.Error(w, "Failed to create audio track or add to the peer connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	logWithTime("[WEBRTC] Setting remote description")
	err = setRemoteDescription(peerConnection, *offer)
	if err != nil {
		logWithTime("[WEBRTC][ERROR] Error setting remote description: %v", err)
		http.Error(w, fmt.Sprintf("Failed to set remote description: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[WEBRTC] Creating answer with transceivers: %+v", peerConnection.GetTransceivers())
	answer, err := createAnswer(peerConnection)
	if err != nil {
		logWithTime("[WEBRTC][ERROR] Error creating answer: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create answer: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[WEBRTC] Answer SDP: %s", answer.SDP)

	logWithTime("[WEBRTC] Finalizing connection setup")
	if err := finalizeConnectionSetup(appSession, audioTrack, *answer); err != nil {
		logWithTime("[WEBRTC][ERROR] Error finalizing connection setup: %v", err)
		http.Error(w, fmt.Sprintf("Failed to finalize connection setup: %v", err), http.StatusInternalServerError)
		return
	}

	logWithTime("[WEBRTC] Sending answer to client")
	sendAnswer(w, peerConnection.LocalDescription())
}

// why we need cloud-optimized timeouts:
// - account for network latency
// - handle various connection paths
// - provide stability in diverse environments
const (
	iceDisconnectedTimeout = 45 * time.Second // Extended for more stability
	iceFailedTimeout       = 60 * time.Second // Extended for more stability
	iceKeepaliveInterval   = 1 * time.Second  // More frequent keepalive
	iceGatheringTimeout    = 30 * time.Second // Extended for better gathering
)

// why we need connection state tracking:
// - ensure clean state between attempts
// - prevent stale candidates
// - improve reconnection reliability
type connectionState struct {
	lastICEState       webrtc.ICEConnectionState
	successfulPairs    int
	lastDisconnectTime time.Time
}

func finalizeConnectionSetup(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample, answer webrtc.SessionDescription) error {
	connState := &connectionState{}

	// Track ICE connection state changes
	appSession.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[ICE] Connection state changed from %s to %s for session %s",
			connState.lastICEState, state, appSession.Id)

		if state == webrtc.ICEConnectionStateChecking {
			stats := appSession.PeerConnection.GetStats()
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidatePairStats); ok && s.State == "succeeded" {
					connState.successfulPairs++
					log.Printf("[ICE] Found successful candidate pair: local=%s, remote=%s (total: %d)",
						s.LocalCandidateID, s.RemoteCandidateID, connState.successfulPairs)
				}
			}
		} else if state == webrtc.ICEConnectionStateDisconnected {
			connState.lastDisconnectTime = time.Now()
			log.Printf("[ICE] Connection disconnected at %v", connState.lastDisconnectTime)
		}

		connState.lastICEState = state
	})

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

	// why we need enhanced monitoring:
	// - detect and recover from connection issues
	// - track audio pipeline health
	// - provide detailed diagnostics
	go func() {
		fastTicker := time.NewTicker(3000 * time.Millisecond)
		defer fastTicker.Stop()

		time.AfterFunc(5*time.Second, func() {
			fastTicker.Stop()
		})

		slowTicker := time.NewTicker(5 * time.Second)
		defer slowTicker.Stop()

		var lastConnectionCheck time.Time
		var consecutiveFailures int

		for {
			select {
			case <-appSession.MonitorDone:
				return
			case <-fastTicker.C:
				checkJACKConnections(appSession)
				checkAudioPipelineHealth(appSession)
			case <-slowTicker.C:
				checkJACKConnections(appSession)
				checkAudioPipelineHealth(appSession)

				// Check WebRTC connection health
				if appSession.PeerConnection != nil {
					if time.Since(lastConnectionCheck) > 10*time.Second {
						lastConnectionCheck = time.Now()

						if state := appSession.PeerConnection.ICEConnectionState(); state == webrtc.ICEConnectionStateDisconnected {
							consecutiveFailures++
							log.Printf("[%s] ICE disconnected (failures: %d)", appSession.Id, consecutiveFailures)

							if consecutiveFailures >= 3 {
								log.Printf("[%s] Attempting connection recovery", appSession.Id)
								go attemptConnectionRecovery(appSession)
								consecutiveFailures = 0
							}
						} else if state == webrtc.ICEConnectionStateConnected {
							consecutiveFailures = 0
						}
					}
				}
			}
		}
	}()

	// Monitor WebRTC stats with enhanced logging
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var lastStats struct {
			packetsLost int32
			jitter      float64
		}

		for range ticker.C {
			if appSession.PeerConnection != nil {
				stats := appSession.PeerConnection.GetStats()
				for _, stat := range stats {
					switch s := stat.(type) {
					case *webrtc.OutboundRTPStreamStats:
						log.Printf("[%s] Outbound RTP: packets=%d bytes=%d",
							appSession.Id, s.PacketsSent, s.BytesSent)
					case *webrtc.InboundRTPStreamStats:
						packetLossDelta := s.PacketsLost - lastStats.packetsLost
						jitterDelta := s.Jitter - lastStats.jitter

						log.Printf("[%s] Inbound RTP: packets=%d bytes=%d jitter=%v packet_loss_delta=%d jitter_delta=%f",
							appSession.Id, s.PacketsReceived, s.BytesReceived, s.Jitter, packetLossDelta, jitterDelta)

						lastStats.packetsLost = s.PacketsLost
						lastStats.jitter = s.Jitter
					}
				}
			}
		}
	}()
}

// why we need pipeline health checks:
// - detect audio pipeline issues early
// - prevent silent failures
// - maintain audio quality
func checkAudioPipelineHealth(appSession *session.AppSession) {
	if appSession.GStreamerPipeline != nil {
		// Check if pipeline exists and restart if needed
		if appSession.GStreamerPipeline.Pipeline == nil {
			log.Printf("[%s] Pipeline not initialized, attempting restart", appSession.Id)
			appSession.GStreamerPipeline.Stop()
			appSession.GStreamerPipeline.Start()
		}
	}
}

// why we need connection recovery:
// - handle temporary network issues
// - maintain session stability
// - prevent unnecessary disconnects
func attemptConnectionRecovery(appSession *session.AppSession) {
	log.Printf("[%s] Starting connection recovery", appSession.Id)

	// Create new ICE candidates by setting local description again
	if desc := appSession.PeerConnection.LocalDescription(); desc != nil {
		if err := appSession.PeerConnection.SetLocalDescription(*desc); err != nil {
			log.Printf("[%s] Failed to restart ICE: %v", appSession.Id, err)
			return
		}
	}

	// Wait for recovery or timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Printf("[%s] Connection recovery timed out", appSession.Id)
			return
		case <-ticker.C:
			if appSession.PeerConnection.ICEConnectionState() == webrtc.ICEConnectionStateConnected {
				log.Printf("[%s] Connection recovered successfully", appSession.Id)
				return
			}
		}
	}
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

func processOffer(r *http.Request) (*webrtc.SessionDescription, error) {
	var browserOffer BrowserOffer

	log.Println("[OFFER] Decoding offer JSON")
	err := json.NewDecoder(r.Body).Decode(&browserOffer)
	if err != nil {
		log.Printf("[OFFER][ERROR] JSON decode failed: %v", err)
		return nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	offer := webrtc.SessionDescription{}
	signal.Decode(browserOffer.SDP, &offer)

	// Create a MediaEngine and populate it from the SDP
	mediaEngine := webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("failed to register default codecs: %v", err)
	}

	log.Printf("[OFFER] Media direction in offer: %v", offer.SDP)

	// Log detailed SDP analysis
	log.Printf("[OFFER] Decoded SDP details:")
	log.Printf("[OFFER] - Type: %s", offer.Type)
	log.Printf("[OFFER] - SDP Length: %d", len(offer.SDP))
	log.Printf("[OFFER] - SDP Preview: %.100s...", offer.SDP)

	return &offer, nil
}

func setSessionToConnection(w http.ResponseWriter, r *http.Request, peerConnection *webrtc.PeerConnection) (*session.AppSession, error) {
	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		return nil, err
	}
	appSession.PeerConnection = peerConnection

	// why we need connection state monitoring:
	// - detect browser window closes
	// - ensure cleanup on unexpected disconnects
	// - prevent orphaned jack connections
	appSession.PeerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Connection state changed to: %s", state.String())

		// cleanup strategy for webrtc connections:
		// - only log states when the connection is still valid
		// - avoid redundant logging in terminal states
		// - ensure proper cleanup of resources when connection ends
		// - prevent memory leaks from orphaned sessions

		// only log connection details if peer connection is still valid
		if appSession.PeerConnection != nil {
			if sigState := appSession.PeerConnection.SignalingState(); sigState != webrtc.SignalingStateClosed {
				log.Printf("Signaling State: %s", sigState.String())
			}
			// avoid checking connection state if we're already in a terminal state
			if state != webrtc.PeerConnectionStateClosed &&
				state != webrtc.PeerConnectionStateFailed {
				log.Printf("Connection State: %s", state.String())
			}
		}

		// Clean up when connection is closed or failed
		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {
			log.Printf("[CLEANUP] Connection state %s triggered cleanup for session %s", state, appSession.Id)
			appSession.StopAllProcesses()
		}
	})

	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candidateStr := candidate.String()
			log.Printf("[ICE] Processing candidate: %s", candidateStr)

			// Only send candidates after remote description is set
			if peerConnection.RemoteDescription() == nil {
				log.Printf("[ICE] Waiting for remote description before sending candidate")
				return
			}

			// Log candidate details for monitoring
			log.Printf("[ICE] Processing candidate: protocol=%s address=%s port=%d priority=%d type=%s",
				candidate.Protocol,
				candidate.Address,
				candidate.Port,
				candidate.Priority,
				candidateStr)
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
func createPeerConnection(iceServers []webrtc.ICEServer, sessionID string) (*webrtc.PeerConnection, error) {
	logWithTime("[WEBRTC] Creating peer connection for session: %s", sessionID)
	s := webrtc.SettingEngine{}

	s.SetIncludeLoopbackCandidate(false)
	s.SetNAT1To1IPs([]string{}, webrtc.ICECandidateTypeHost)
	logWithTime("[ICE] Configured NAT and candidate settings")

	s.SetHostAcceptanceMinWait(500 * time.Millisecond)
	s.SetSrflxAcceptanceMinWait(1000 * time.Millisecond)
	s.SetRelayAcceptanceMinWait(2000 * time.Millisecond)
	logWithTime("[ICE] Set candidate acceptance delays")

	username, password := getICECredentials()
	s.SetICECredentials(username, password)
	logWithTime("[ICE] Set ICE credentials")

	s.SetLite(false)
	logWithTime("[ICE] ICE-Lite mode disabled for proper candidate gathering")

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		logWithTime("[MEDIA][ERROR] Failed to register codecs: %v", err)
		return nil, fmt.Errorf("failed to register codecs: %v", err)
	}
	logWithTime("[MEDIA] Registered default codecs")

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
	)
	logWithTime("[WEBRTC] Created API with custom settings")

	config := webrtc.Configuration{
		ICEServers:           iceServers,
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		ICECandidatePoolSize: 1,
		RTCPMuxPolicy:        webrtc.RTCPMuxPolicyRequire,
		// why we need ICETransportPolicyRelay:
		// - ensures consistent behavior between local and production
		// - required for container networking in production (ECS/Fargate)
		// - prevents direct/host candidates that won't work in cloud
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	}
	logWithTime("[WEBRTC] Created configuration: %+v", config)

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		logWithTime("[WEBRTC][ERROR] Failed to create peer connection: %v", err)
		return nil, err
	}
	logWithTime("[WEBRTC] Created peer connection successfully")

	// Add connection state monitoring
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logWithTime("[WEBRTC] Connection state changed to %s", state.String())
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		logWithTime("[ICE] Connection state changed to %s", state.String())
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

// why we need robust ice candidate handling:
// - ensures reliable peer connections
// - handles network changes gracefully
// - improves connection stability
func HandleICECandidate(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	logWithTime("[ICE] Received candidate for session: %s", sessionID)

	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		logWithTime("[ICE][ERROR] Session not found: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if appSession.PeerConnection == nil {
		logWithTime("[ICE][ERROR] No peer connection for session %s", sessionID)
		http.Error(w, "No peer connection", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logWithTime("[ICE][ERROR] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logWithTime("[ICE] Raw request body: %s", string(body))

	var wrapper struct {
		Candidate string `json:"candidate"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&wrapper); err != nil {
		logWithTime("[ICE][ERROR] Failed to decode outer wrapper: %v", err)
		logWithTime("[ICE][DEBUG] Attempted to decode body: %s", string(body))
		http.Error(w, "Invalid candidate format", http.StatusBadRequest)
		return
	}

	candidateJSON, err := base64.StdEncoding.DecodeString(wrapper.Candidate)
	if err != nil {
		logWithTime("[ICE][ERROR] Failed to decode base64: %v", err)
		logWithTime("[ICE][DEBUG] Attempted to decode base64: %s", wrapper.Candidate)
		http.Error(w, "Invalid base64 encoding", http.StatusBadRequest)
		return
	}

	var candidateObj struct {
		Candidate        string `json:"candidate"`
		SDPMid           string `json:"sdpMid"`
		SDPMLineIndex    uint16 `json:"sdpMLineIndex"`
		UsernameFragment string `json:"usernameFragment"`
	}
	if err := json.Unmarshal(candidateJSON, &candidateObj); err != nil {
		logWithTime("[ICE][ERROR] Failed to decode candidate JSON: %v", err)
		logWithTime("[ICE][DEBUG] Attempted to decode JSON: %s", string(candidateJSON))
		http.Error(w, "Invalid candidate format", http.StatusBadRequest)
		return
	}

	logWithTime("[ICE] Processing candidate: %+v", candidateObj)

	// Log candidate type but accept both STUN and host candidates in ECS
	if strings.Contains(candidateObj.Candidate, "typ srflx") {
		log.Printf("[ICE] Processing STUN candidate: %s", candidateObj.Candidate)
	} else if strings.Contains(candidateObj.Candidate, "typ host") {
		log.Printf("[ICE] Processing host candidate: %s", candidateObj.Candidate)
	} else if strings.Contains(candidateObj.Candidate, "typ relay") {
		log.Printf("[ICE] Processing relay candidate: %s", candidateObj.Candidate)
	} else {
		log.Printf("[ICE] Processing other candidate type: %s", candidateObj.Candidate)
	}

	candidate := webrtc.ICECandidateInit{
		Candidate:        candidateObj.Candidate,
		SDPMid:           &candidateObj.SDPMid,
		SDPMLineIndex:    &candidateObj.SDPMLineIndex,
		UsernameFragment: &candidateObj.UsernameFragment,
	}

	if err := appSession.PeerConnection.AddICECandidate(candidate); err != nil {
		logWithTime("[ICE][ERROR] Failed to add candidate: %v", err)
		logWithTime("[ICE][DEBUG] Failed candidate details: %+v", candidate)
		http.Error(w, fmt.Sprintf("Failed to add candidate: %v", err), http.StatusInternalServerError)
		return
	}
	logWithTime("[ICE][SUCCESS] Added STUN candidate for session %s", sessionID)

	w.WriteHeader(http.StatusOK)
}

func logWithTime(format string, v ...interface{}) {
	log.Printf("[%s] %s", time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), fmt.Sprintf(format, v...))
}
