package webrtc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	// Generate a secure password that is at least 128 bits (32 characters) long
	// This matches the TURN server config and meets WebRTC security requirements
	username := "awestruck_user"
	password := "verySecurePassword1234567890abcdefghijklmnop"
	return username, password
}

// why we need consistent port ranges:
// - matches turn server configuration (49152-49252)
// - ensures reliable ice candidate generation
// - prevents permission errors from mismatched ports
func getICEServers() []webrtc.ICEServer {
	hostname := config.GetTurnServer()
	username, password := getICECredentials()

	// Always use relay-only configuration for consistent testing
	return []webrtc.ICEServer{
		{
			URLs: []string{
				fmt.Sprintf("turn:%s:3478?transport=udp", hostname),
			},
			Username:   username,
			Credential: password,
		},
	}
}

// why we need webrtc settings:
// - configures global webrtc behavior
// - sets up ice candidate filtering
// - ensures consistent port usage with turn server
func configureWebRTC() (*webrtc.API, error) {
	s := webrtc.SettingEngine{}

	// Configure port range to match TURN server
	s.SetEphemeralUDPPortRange(49152, 49252)

	// Disable ICE-Lite mode for proper candidate gathering
	s.SetLite(false)

	// Create media engine with default codecs
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("failed to register codecs: %v", err)
	}

	// Create WebRTC API with settings
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
	)

	return api, nil
}

// why we need a config endpoint:
// - provides ice configuration to client
// - ensures consistent settings
// - enables environment-specific config
func HandleConfig(w http.ResponseWriter, r *http.Request) {
	// Force relay for all environments for consistent testing
	config := webrtc.Configuration{
		ICEServers:           getICEServers(),
		ICETransportPolicy:   webrtc.ICETransportPolicyRelay,
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:        webrtc.RTCPMuxPolicyRequire,
		ICECandidatePoolSize: 1,
	}

	// Add port range info to client config
	configJSON := struct {
		webrtc.Configuration
		PortRange struct {
			Min uint16 `json:"min"`
			Max uint16 `json:"max"`
		} `json:"portRange"`
	}{
		Configuration: config,
		PortRange: struct {
			Min uint16 `json:"min"`
			Max uint16 `json:"max"`
		}{
			Min: 49152,
			Max: 49252,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configJSON)
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

// why we need enhanced audio pipeline monitoring:
// - detect if audio is flowing from jack to gstreamer
// - ensure proper sample rate conversion
// - monitor audio levels before encoding
func startMediaPipeline(appSession *session.AppSession, audioTrack *webrtc.TrackLocalStaticSample) error {
	pipelineReady := make(chan struct{})

	go func() {
		log.Println("Creating pipeline...")

		// Add monitoring but keep pipeline simple
		// pipelineConfig := fmt.Sprintf("%s ! "+
		// 	"level name=audiolevel interval=1000000000 post-messages=true ! "+
		// 	"audioconvert ! audioresample quality=10 ! "+
		// 	"level name=outputlevel interval=1000000000 post-messages=true ! "+
		// 	"opusenc frame-size=20 complexity=10 bitrate=128000", *appSession.AudioSrc)
		pipelineConfig := *appSession.AudioSrc

		log.Printf("Using enhanced pipeline config: %s", pipelineConfig)

		appSession.GStreamerPipeline = gst.CreatePipeline("opus", []*webrtc.TrackLocalStaticSample{audioTrack}, pipelineConfig)

		if appSession.GStreamerPipeline == nil {
			log.Printf("[%s] Failed to create pipeline", appSession.Id)
			return
		}

		// Start monitoring audio levels
		go monitorAudioLevels(appSession)

		appSession.GStreamerPipeline.Start()
		log.Printf("[%s] Pipeline created and started", appSession.Id)
		close(pipelineReady)
	}()

	select {
	case <-pipelineReady:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for pipeline to start")
	}
}

// why we need enhanced audio level monitoring:
// - track audio flow through pipeline stages
// - detect silence or low levels
// - identify pipeline stalls
func monitorAudioLevels(appSession *session.AppSession) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if appSession.GStreamerPipeline == nil || appSession.GStreamerPipeline.Pipeline == nil {
			return
		}

		// Get stats from pipeline and WebRTC
		stats := appSession.PeerConnection.GetStats()
		var audioStats struct {
			packetsReceived uint32
			bytesReceived   uint64
			packetsLost     int32
		}

		for _, stat := range stats {
			if s, ok := stat.(*webrtc.InboundRTPStreamStats); ok {
				audioStats.packetsReceived += s.PacketsReceived
				audioStats.bytesReceived += s.BytesReceived
				audioStats.packetsLost += s.PacketsLost
			}
		}

		log.Printf("[%s][AUDIO] Stats - Packets: rx=%d lost=%d, Bytes: %d",
			appSession.Id,
			audioStats.packetsReceived,
			audioStats.packetsLost,
			audioStats.bytesReceived)
	}
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

	// why we need detailed offer logging:
	// - helps debug encoding/decoding issues
	// - tracks sdp transformation through system
	// - identifies protocol mismatches
	log.Printf("[OFFER] Processing new offer from client")

	log.Printf("[OFFER] Reading request body")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[OFFER][ERROR] Failed to read request body: %v", err)
		return nil, fmt.Errorf("failed to read request body: %v", err)
	}
	log.Printf("[OFFER] Raw request body: %s", string(body))

	log.Printf("[OFFER] Decoding offer JSON")
	err = json.Unmarshal(body, &browserOffer)
	if err != nil {
		log.Printf("[OFFER][ERROR] JSON decode failed: %v", err)
		return nil, fmt.Errorf("failed to decode JSON: %v", err)
	}
	log.Printf("[OFFER] Decoded browser offer: %+v", browserOffer)

	offer := webrtc.SessionDescription{}
	log.Printf("[OFFER] Decoding base64 SDP")

	// why we need panic recovery:
	// - signal.Decode panics on error
	// - ensures graceful error handling
	// - provides detailed error logs
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[OFFER][ERROR] SDP decode panicked: %v", r)
				err = fmt.Errorf("SDP decode failed: %v", r)
			}
		}()
		signal.Decode(browserOffer.SDP, &offer)
	}()
	if err != nil {
		return nil, err
	}
	log.Printf("[OFFER] Decoded SDP: %+v", offer)

	// Create a MediaEngine and populate it from the SDP
	mediaEngine := webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		log.Printf("[OFFER][ERROR] Failed to register default codecs: %v", err)
		return nil, fmt.Errorf("failed to register default codecs: %v", err)
	}

	// Log detailed SDP analysis
	log.Printf("[OFFER] Detailed SDP analysis:")
	log.Printf("[OFFER] - Type: %s", offer.Type)
	log.Printf("[OFFER] - SDP Length: %d", len(offer.SDP))
	log.Printf("[OFFER] - Full SDP:\n%s", offer.SDP)

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

// why we need consistent peer connection setup:
// - ensures proper ice configuration
// - sets up port ranges and transport policies
// - enables reliable webrtc connections
func createPeerConnection(iceServers []webrtc.ICEServer, sessionID string) (*webrtc.PeerConnection, error) {
	// Get WebRTC API with configured settings
	api, err := configureWebRTC()
	if err != nil {
		return nil, fmt.Errorf("failed to configure WebRTC: %v", err)
	}

	// Keep configuration minimal like the working example
	config := webrtc.Configuration{
		ICEServers: iceServers,
		// ICETransportPolicy: webrtc.ICETransportPolicyRelay,
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %v", err)
	}

	// Monitor ICE gathering state with enhanced logging
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		logWithTime("[ICE][%s] Gathering state changed to %s", sessionID, state.String())

		// Log current candidates when gathering is complete
		if state == webrtc.ICEGathererStateComplete {
			stats := pc.GetStats()
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidateStats); ok {
					logWithTime("[ICE][%s] Final candidate: type=%s protocol=%s address=%s:%d priority=%d",
						sessionID, s.CandidateType, s.Protocol, s.IP, s.Port, s.Priority)
				}
			}
		}
	})

	// Enhanced ICE connection state monitoring
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		logWithTime("[ICE][%s] Connection state changed to %s", sessionID, state.String())

		if state == webrtc.ICEConnectionStateChecking {
			logWithTime("[ICE][%s] Starting connectivity checks", sessionID)

			// Log candidate pairs being checked
			stats := pc.GetStats()
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidatePairStats); ok {
					logWithTime("[ICE][%s] Checking pair: local=%s remote=%s state=%s nominated=%v",
						sessionID, s.LocalCandidateID, s.RemoteCandidateID, s.State, s.Nominated)

					// Log detailed candidate information
					for _, stat2 := range stats {
						if c, ok := stat2.(*webrtc.ICECandidateStats); ok {
							if c.ID == s.LocalCandidateID {
								logWithTime("[ICE][%s] Local candidate: type=%s protocol=%s address=%s:%d relayProtocol=%s",
									sessionID, c.CandidateType, c.Protocol, c.IP, c.Port, c.RelayProtocol)
							}
							if c.ID == s.RemoteCandidateID {
								logWithTime("[ICE][%s] Remote candidate: type=%s protocol=%s address=%s:%d relayProtocol=%s",
									sessionID, c.CandidateType, c.Protocol, c.IP, c.Port, c.RelayProtocol)
							}
						}
					}
				}
			}
		} else if state == webrtc.ICEConnectionStateConnected {
			// Log selected candidate pair when connection is established
			stats := pc.GetStats()
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidatePairStats); ok && s.Nominated {
					logWithTime("[ICE][%s] Selected pair: local=%s remote=%s state=%s",
						sessionID, s.LocalCandidateID, s.RemoteCandidateID, s.State)
				}
			}
		}
	})

	// Monitor individual ICE candidates with detailed logging
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			logWithTime("[ICE][%s] New candidate: %s (type: %s, protocol: %s)",
				sessionID, c.String(), c.Typ, c.Protocol)

			// Log additional details for relay candidates
			if c.Typ == webrtc.ICECandidateTypeRelay {
				logWithTime("[ICE][%s] Relay details - address: %s:%d related: %s:%d raddr: %s rport: %d",
					sessionID, c.Address, c.Port, c.RelatedAddress, c.RelatedPort, c.RelatedAddress, c.RelatedPort)
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
	// why we need detailed answer logging:
	// - helps debug encoding/decoding issues
	// - tracks sdp transformation through system
	// - identifies protocol mismatches
	log.Printf("[ANSWER] Preparing to send answer")
	log.Printf("[ANSWER] Raw answer: %+v", answer)
	log.Printf("[ANSWER] SDP content:\n%s", answer.SDP)

	// why we need base64 encoding:
	// - matches client expectations
	// - ensures safe transport of sdp
	// - maintains protocol compatibility
	encodedSDP := signal.Encode(answer)
	log.Printf("[ANSWER] Base64 encoded SDP: %s", encodedSDP)

	response := BrowserOffer{
		SDP:  encodedSDP,
		Type: answer.Type.String(),
	}

	answerJSON, err := json.Marshal(response)
	if err != nil {
		log.Printf("[ANSWER][ERROR] Failed to encode answer: %v", err)
		http.Error(w, "Failed to encode answer", http.StatusInternalServerError)
		return
	}

	log.Printf("[ANSWER] Final JSON response: %s", string(answerJSON))
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

// why we need turn permission handling:
// - ensures proper relay setup
// - manages peer permissions
// - improves connection reliability
func HandleTURNPermission(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	logWithTime("[TURN] Received permission request for session: %s", sessionID)

	var request struct {
		Address   string `json:"address"`
		Port      int    `json:"port"`
		Protocol  string `json:"protocol"`
		IsRelay   bool   `json:"isRelay"`
		LocalAddr string `json:"localAddr"` // Client can provide its local address
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logWithTime("[TURN][ERROR] Failed to decode permission request: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	appSession, err := session.GetOrCreateSession(r, w)
	if err != nil {
		logWithTime("[TURN][ERROR] Session not found: %v", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if appSession.PeerConnection == nil {
		logWithTime("[TURN][ERROR] No peer connection for session %s", sessionID)
		http.Error(w, "No peer connection", http.StatusBadRequest)
		return
	}

	var localAddr string

	// First try using the local address provided by the client
	if request.LocalAddr != "" {
		localAddr = request.LocalAddr
		logWithTime("[TURN][DEBUG] Using client-provided local address: %s", localAddr)
	}

	// If no client-provided address, try to get it from peer connection stats
	if localAddr == "" {
		stats := appSession.PeerConnection.GetStats()

		// Log all ICE candidate stats for debugging
		for _, stat := range stats {
			if s, ok := stat.(*webrtc.ICECandidateStats); ok {
				logWithTime("[TURN][DEBUG] Found candidate: type=%s, ip=%s, port=%d, protocol=%s",
					s.CandidateType, s.IP, s.Port, s.Protocol)
			}
		}

		// Try to find a relay candidate first
		for _, stat := range stats {
			if s, ok := stat.(*webrtc.ICECandidateStats); ok {
				if s.CandidateType == webrtc.ICECandidateTypeRelay && s.Protocol == "udp" {
					localAddr = fmt.Sprintf("%s:%d", s.IP, s.Port)
					logWithTime("[TURN][DEBUG] Using relay candidate: %s", localAddr)
					break
				}
			}
		}

		// If no relay candidate found, try host candidate as fallback
		if localAddr == "" {
			for _, stat := range stats {
				if s, ok := stat.(*webrtc.ICECandidateStats); ok {
					if s.CandidateType == webrtc.ICECandidateTypeHost && s.Protocol == "udp" {
						localAddr = fmt.Sprintf("%s:%d", s.IP, s.Port)
						logWithTime("[TURN][DEBUG] Using host candidate: %s", localAddr)
						break
					}
				}
			}
		}
	}

	if localAddr == "" {
		logWithTime("[TURN][ERROR] Could not find local address for session %s", sessionID)
		http.Error(w, "Could not find local address", http.StatusInternalServerError)
		return
	}

	// Forward permission request to TURN server
	turnAddr := "192.168.4.82:3479"

	// Create permission request
	permReq := struct {
		ClientAddr string `json:"client_addr"`
		PeerAddr   string `json:"peer_addr"`
		Protocol   string `json:"protocol"`
		SessionID  string `json:"session_id"` // Add session ID to request body
	}{
		ClientAddr: localAddr,
		PeerAddr:   fmt.Sprintf("%s:%d", request.Address, request.Port),
		Protocol:   request.Protocol,
		SessionID:  sessionID, // Include session ID from request header
	}

	// Send request to TURN server's permission endpoint
	permReqBytes, err := json.Marshal(permReq)
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to marshal permission request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logWithTime("[TURN][DEBUG] Sending permission request: %+v", permReq)
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/permission", turnAddr), bytes.NewBuffer(permReqBytes))
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to create permission request: %v", err)
		http.Error(w, "Failed to create permission request", http.StatusInternalServerError)
		return
	}

	// Add session ID to request headers
	req.Header.Set("X-Session-ID", sessionID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logWithTime("[TURN][ERROR] Failed to forward permission request: %v", err)
		http.Error(w, "Failed to create permission", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		logWithTime("[TURN][ERROR] TURN server rejected permission request: %s", string(body))
		http.Error(w, "Permission request rejected", resp.StatusCode)
		return
	}

	logWithTime("[TURN] Successfully created permission for %s:%d", request.Address, request.Port)
	w.WriteHeader(http.StatusOK)
}

func logWithTime(format string, v ...interface{}) {
	log.Printf("[%s] %s", time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), fmt.Sprintf(format, v...))
}

// why we need file watching:
// - enables hot reloading
// - updates last-modified headers
// - supports continuous testing
func watchClientFiles() {
	clientDir := "./client"
	log.Printf("[WATCH] Starting file watcher for %s", clientDir)

	for {
		time.Sleep(time.Second)
		// Touch the files to update last-modified time
		files, err := ioutil.ReadDir(clientDir)
		if err != nil {
			log.Printf("[WATCH][ERROR] Failed to read client directory: %v", err)
			continue
		}

		for _, file := range files {
			if !file.IsDir() {
				path := filepath.Join(clientDir, file.Name())
				currentTime := time.Now()
				err := os.Chtimes(path, currentTime, currentTime)
				if err != nil {
					log.Printf("[WATCH][ERROR] Failed to update file time for %s: %v", path, err)
				}
			}
		}
	}
}

func init() {
	// Start file watcher in background
	go watchClientFiles()
}

// why we need client log handling:
// - captures client-side errors
// - helps debug webrtc issues
// - provides visibility into browser state
func HandleClientLog(w http.ResponseWriter, r *http.Request) {
	var logData struct {
		Timestamp string `json:"timestamp"`
		Message   string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&logData); err != nil {
		log.Printf("[CLIENT-LOG][ERROR] Failed to decode log: %v", err)
		http.Error(w, "Invalid log format", http.StatusBadRequest)
		return
	}

	log.Printf("[CLIENT] %s %s", logData.Timestamp, logData.Message)
	w.WriteHeader(http.StatusOK)
}
