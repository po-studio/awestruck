// why we need ice progress monitoring:
// - tracks candidate gathering and validation
// - helps diagnose connection issues
// - provides feedback for debugging
function monitorIceProgress() {
    let candidatesSent = 0;
    let candidatesAcknowledged = 0;
    
    return {
        trackCandidate: () => {
            candidatesSent++;
            console.log(`[ICE] Candidates tracked: ${candidatesSent} sent, ${candidatesAcknowledged} acknowledged`);
        },
        trackAcknowledgement: () => {
            candidatesAcknowledged++;
            console.log(`[ICE] Candidates tracked: ${candidatesSent} sent, ${candidatesAcknowledged} acknowledged`);
        },
        getStats: () => ({
            sent: candidatesSent,
            acknowledged: candidatesAcknowledged
        })
    };
}

const iceProgress = monitorIceProgress();

// why we need unique session ids:
// - ensures clean state for each page load
// - prevents resource conflicts
// - enables proper cleanup tracking
function getSessionID() {
    const sessionID = `sid_${Math.random().toString(36).slice(2, 11)}_${Date.now()}`;
    sessionStorage.setItem('sessionID', sessionID);
    return sessionID;
}
const sessionID = getSessionID();

// Globals
let pc = null;
let audioContext = null;
let audioElement = null;
let codePollingInterval = null;
let qualityMonitorInterval = null;
let iceCheckingTimeout = null;
let audioStatsInterval = null;
let audioLevelsInterval = null;
let trackStatsInterval = null;
let lastConnectionState = null;
let pendingCandidates = [];
let remoteDescriptionSet = false;

// why we need connection state tracking:
// - ensure clean state between attempts
// - prevent stale candidates
// - improve reconnection reliability
let connectionState = {
    lastState: null,
    lastStateChangeTime: null,
    successfulPairs: 0,
    gatheringComplete: false,
    lastDisconnectTime: null
};

// Add session ID to all HTMX requests
document.body.addEventListener('htmx:configRequest', evt => {
  evt.detail.headers['X-Session-ID'] = sessionID;
});

// Handle a response from the server (e.g., after an HTMX request)
async function handleSynthResponse(event) {
  const button = document.querySelector('.connection-button');

  if (event.detail.failed) {
      button.textContent = 'Error - Try Again';
      return;
  }

  // If there's an existing connection, stop it
  if (pc) {
      await cleanupConnection();
      button.textContent = 'Generate Synth';
      button.classList.remove('button-disconnect');
      return;
  }

  button.textContent = 'Connecting...';
  button.disabled = true;

  try {
      await setupWebRTC();
      button.textContent = 'Stop Synth';
      button.classList.add('button-disconnect');
      button.disabled = false;
  } catch (error) {
      console.error('Connection failed:', error);
      button.textContent = 'Error - Try Again';
      button.disabled = false;
  }
}

// why we need ice connection monitoring:
// - ensures we wait for successful ICE connection before proceeding
// - helps diagnose connection failures early
// - provides timing information for debugging
function waitForICEConnection(pc) {
    return new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
            reject(new Error('ICE connection timeout'));
        }, 30000); // 30 seconds to match server gathering timeout

        function checkState() {
            if (pc.iceConnectionState === 'connected' || 
                pc.iceConnectionState === 'completed') {
                clearTimeout(timeout);
                resolve();
            } else if (pc.iceConnectionState === 'failed' ||
                      pc.iceConnectionState === 'disconnected' ||
                      pc.iceConnectionState === 'closed') {
                clearTimeout(timeout);
                reject(new Error(`ICE connection failed: ${pc.iceConnectionState}`));
            }
        }

        pc.addEventListener('iceconnectionstatechange', checkState);
        checkState();
    });
}

// why we need dynamic ice configuration:
// - server controls ice settings
// - supports environment changes
// - ensures consistent behavior
async function getICEConfig() {
    const response = await fetch('/config');
    if (!response.ok) {
        throw new Error('Failed to fetch ICE configuration');
    }
    const config = await response.json();
    
    // Force relay mode and ensure it's not overridden
    config.iceTransportPolicy = 'relay';
    
    // Add detailed logging of ICE configuration
    console.log('[ICE] Configuration:', {
        iceTransportPolicy: config.iceTransportPolicy,
        iceServers: config.iceServers.map(server => ({
            urls: server.urls,
            username: server.username,
            hasCredential: !!server.credential
        })),
        bundlePolicy: config.bundlePolicy,
        rtcpMuxPolicy: config.rtcpMuxPolicy,
        iceCandidatePoolSize: config.iceCandidatePoolSize
    });
    
    return config;
}

// why we need webrtc setup:
// - initializes peer connection with server config
// - sets up media tracks and data channels
// - handles connection state changes
async function setupWebRTC() {
    const config = await getICEConfig();
    console.log('[ICE] Using server configuration:', config);

    pc = new RTCPeerConnection(config);
    console.log('[WebRTC] Created peer connection with config:', config);

    // why we need enhanced dtls monitoring:
    // - tracks handshake progress in detail
    // - helps identify connection issues
    // - provides timing information
    const monitorDTLS = () => {
        console.log('[DTLS] Starting monitoring');
        const startTime = Date.now();
        
        const checkDTLS = async () => {
            const stats = await pc.getStats();
            let dtlsFound = false;
            
            stats.forEach(stat => {
                if (stat.type === 'transport') {
                    dtlsFound = true;
                    console.log('[DTLS] Transport stats:', {
                        state: stat.dtlsState,
                        timeSinceStart: (Date.now() - startTime) / 1000,
                        localCertificate: !!stat.localCertificateId,
                        remoteCertificate: !!stat.remoteCertificateId,
                        selectedCandidatePairId: stat.selectedCandidatePairId
                    });
                    
                    if (stat.dtlsState === 'new') {
                        console.warn('[DTLS] Still in new state after', (Date.now() - startTime) / 1000, 'seconds');
                    }
                }
                
                if (stat.type === 'candidate-pair') {
                    console.log('[ICE] Candidate pair:', {
                        state: stat.state,
                        nominated: stat.nominated,
                        bytesSent: stat.bytesSent,
                        bytesReceived: stat.bytesReceived,
                        totalRoundTripTime: stat.totalRoundTripTime
                    });
                }
            });
            
            if (!dtlsFound) {
                console.warn('[DTLS] No transport stats found');
            }
            
            // Continue monitoring if still in 'new' state
            if (pc.connectionState === 'checking' || pc.connectionState === 'connecting') {
                setTimeout(checkDTLS, 1000);
            }
        };
        
        checkDTLS();
    };

    // why we need connection state tracking:
    // - provides detailed state transitions
    // - helps debug connection issues
    // - monitors ice and dtls progress
    pc.onconnectionstatechange = () => {
        console.log('[WebRTC] Connection state changed:', {
            connectionState: pc.connectionState,
            iceConnectionState: pc.iceConnectionState,
            iceGatheringState: pc.iceGatheringState,
            signalingState: pc.signalingState
        });
        
        if (pc.connectionState === 'failed') {
            console.error('[WebRTC] Connection failed - gathering diagnostic info');
            pc.getStats().then(stats => {
                stats.forEach(stat => {
                    if (stat.type === 'candidate-pair' || stat.type === 'transport') {
                        console.log(`[WebRTC] ${stat.type} stats:`, stat);
                    }
                });
            });
        }
    };

    // why we need audio track handling:
    // - sets up audio playback when track is received
    // - monitors track state changes
    // - provides detailed debugging info
    pc.ontrack = (event) => {
        console.log('[AUDIO] Received track:', {
            kind: event.track.kind,
            id: event.track.id,
            enabled: event.track.enabled,
            muted: event.track.muted,
            readyState: event.track.readyState,
            constraints: event.track.getConstraints(),
            settings: event.track.getSettings()
        });

        if (event.track.kind === 'audio') {
            setupAudioElement(event.track);
            
            // Monitor track state changes
            event.track.onended = () => console.log('[AUDIO] Track ended');
            event.track.onmute = () => console.log('[AUDIO] Track muted');
            event.track.onunmute = () => console.log('[AUDIO] Track unmuted');
            
            // Add audio processing debugging
            try {
                audioContext = new (window.AudioContext || window.webkitAudioContext)();
                console.log('[AUDIO] Created AudioContext:', {
                    sampleRate: audioContext.sampleRate,
                    state: audioContext.state,
                    baseLatency: audioContext.baseLatency,
                    outputLatency: audioContext.outputLatency
                });
                
                const mediaStream = new MediaStream([event.track]);
                const source = audioContext.createMediaStreamSource(mediaStream);
                const analyser = audioContext.createAnalyser();
                analyser.fftSize = 2048;
                analyser.smoothingTimeConstant = 0.8;
                
                // Connect source to both analyser and destination
                source.connect(analyser);
                source.connect(audioContext.destination);
                
                // Monitor audio levels using analyser
                const dataArray = new Float32Array(analyser.frequencyBinCount);
                const checkAudioLevels = () => {
                    if (audioContext && audioContext.state === 'running') {
                        analyser.getFloatTimeDomainData(dataArray);
                        let maxLevel = 0;
                        let rms = 0;
                        
                        // Calculate both peak and RMS levels
                        for (let i = 0; i < dataArray.length; i++) {
                            maxLevel = Math.max(maxLevel, Math.abs(dataArray[i]));
                            rms += dataArray[i] * dataArray[i];
                        }
                        rms = Math.sqrt(rms / dataArray.length);
                        
                        console.log('[AUDIO][LEVELS]', {
                            peak: maxLevel.toFixed(4),
                            rms: rms.toFixed(4),
                            frequency: analyser.context.sampleRate,
                            contextTime: audioContext.currentTime,
                            trackState: {
                                enabled: event.track.enabled,
                                muted: event.track.muted,
                                readyState: event.track.readyState
                            }
                        });
                    }
                };
                audioLevelsInterval = setInterval(checkAudioLevels, 500);
            } catch (error) {
                console.error('[AUDIO] Failed to setup audio processing:', error);
            }
        }
    };

    // Create and send offer
    const offer = await pc.createOffer({
        offerToReceiveAudio: true,
        offerToReceiveVideo: false
    });
    
    await pc.setLocalDescription(offer);
    console.log('[WebRTC] Created and set local description');

    await sendOffer(offer);

    pc.onicegatheringstatechange = () => {
        console.log('[ICE] Gathering state changed:', pc.iceGatheringState);
    };

    // why we need enhanced ice monitoring:
    // - track connectivity check patterns
    // - detect stalled checks early
    // - identify relay issues
    pc.oniceconnectionstatechange = () => {
        const state = {
            iceConnectionState: pc.iceConnectionState,
            connectionState: pc.connectionState,
            signalingState: pc.signalingState,
            time: new Date().toISOString()
        };
        console.log('[ICE] Connection state changed:', state);
        
        // Start detailed monitoring on checking state
        if (pc.iceConnectionState === 'checking') {
            monitorICEProgress();
        }
    };

    // why we need detailed ice progress monitoring:
    // - track individual connectivity checks
    // - measure request/response patterns
    // - detect relay issues
    const monitorICEProgress = async () => {
        console.log('[ICE] Starting detailed progress monitoring');
        const startTime = Date.now();
        let lastRequestCount = 0;
        let noResponseDuration = 0;
        
        const checkProgress = async () => {
            const stats = await pc.getStats();
            let foundActivePair = false;
            
            stats.forEach(stat => {
                if (stat.type === 'candidate-pair') {
                    console.log('[ICE] Candidate pair check:', {
                        state: stat.state,
                        requestsSent: stat.requestsSent,
                        responsesReceived: stat.responsesReceived,
                        requestsReceived: stat.requestsReceived,
                        responsesSent: stat.responsesSent,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent,
                        lastRequestTimestamp: stat.lastRequestTimestamp,
                        lastResponseTimestamp: stat.lastResponseTimestamp,
                        totalRoundTripTime: stat.totalRoundTripTime,
                        elapsedTime: (Date.now() - startTime) / 1000
                    });

                    if (stat.state === 'in-progress' || stat.state === 'waiting') {
                        foundActivePair = true;
                        
                        // Check if we're sending requests but not getting responses
                        if (stat.requestsSent > lastRequestCount && stat.responsesReceived === 0) {
                            noResponseDuration += 1;
                            if (noResponseDuration >= 5) {
                                console.warn('[ICE] No responses received for 5 seconds despite sending requests');
                            }
                        } else {
                            noResponseDuration = 0;
                        }
                        lastRequestCount = stat.requestsSent;
                    }
                }
            });
            
            // Continue monitoring while we have active pairs
            if (foundActivePair && pc.iceConnectionState === 'checking') {
                setTimeout(checkProgress, 1000);
            } else {
                console.log('[ICE] Progress monitoring ended:', {
                    finalState: pc.iceConnectionState,
                    duration: (Date.now() - startTime) / 1000
                });
            }
        };
        
        await checkProgress();
    };

    // why we need comprehensive connection monitoring:
    // - tracks both ice and dtls state
    // - provides timing information
    // - helps identify stalled connections
    const monitorConnection = async () => {
        console.log('[CONNECTION] Starting comprehensive monitoring');
        const startTime = Date.now();
        
        const check = async () => {
            const stats = await pc.getStats();
            let foundTransport = false;
            let foundCandidatePair = false;
            
            stats.forEach(stat => {
                if (stat.type === 'transport') {
                    foundTransport = true;
                    console.log('[TRANSPORT] Stats:', {
                        dtlsState: stat.dtlsState,
                        selectedCandidatePairId: stat.selectedCandidatePairId,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent,
                        dtlsRole: stat.dtlsRole,
                        elapsedTime: (Date.now() - startTime) / 1000
                    });
                }
                
                if (stat.type === 'candidate-pair') {
                    foundCandidatePair = true;
                    console.log('[ICE] Pair stats:', {
                        state: stat.state,
                        nominated: stat.nominated,
                        bytesSent: stat.bytesSent,
                        bytesReceived: stat.bytesReceived,
                        totalRoundTripTime: stat.totalRoundTripTime,
                        currentRoundTripTime: stat.currentRoundTripTime,
                        availableOutgoingBitrate: stat.availableOutgoingBitrate,
                        requestsReceived: stat.requestsReceived,
                        requestsSent: stat.requestsSent,
                        responsesReceived: stat.responsesReceived,
                        responsesSent: stat.responsesSent,
                        consentRequestsSent: stat.consentRequestsSent
                    });
                }
            });
            
            if (!foundTransport) {
                console.warn('[TRANSPORT] No transport stats found');
            }
            if (!foundCandidatePair) {
                console.warn('[ICE] No candidate pair stats found');
            }
            
            // Continue monitoring while connection is being established
            if (pc.iceConnectionState !== 'connected' && pc.iceConnectionState !== 'failed') {
                setTimeout(check, 1000);
            }
        };
        
        await check();
    };

    // Start monitoring immediately after PC creation
    monitorConnection();
}

async function sendOffer(offer) {
    try {
        const iceServers = pc.getConfiguration().iceServers;
        console.log('Using WebRTC config:', pc.getConfiguration());

        // Convert the SDP to base64 properly
        const browserOffer = {
            sdp: btoa(JSON.stringify({
                type: offer.type,
                sdp: offer.sdp
            })),
            type: offer.type,
            iceServers: iceServers
        };

        const response = await fetch('/offer', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Session-ID': sessionID
            },
            body: JSON.stringify(browserOffer)
        });

        if (!response.ok) {
            throw new Error('Failed to send offer');
        }

        const answer = await response.json();
        console.log('[WebRTC] Received answer:', answer);
        
        await pc.setRemoteDescription(answer);
        console.log('[WebRTC] Set remote description');

        // Send any pending candidates
        if (pendingCandidates.length > 0) {
            console.log('[ICE] Sending pending candidates:', pendingCandidates.length);
            for (const candidate of pendingCandidates) {
                pc.onicecandidate({ candidate });
            }
            pendingCandidates = [];
        }

        // Wait for ICE connection
        await waitForICEConnection(pc);
        console.log('[WebRTC] ICE connection established');
    } catch (error) {
        console.error('Connection failed:', error);
        throw error;
    }
}

// why we need connection state monitoring:
// - tracks overall WebRTC connection health
// - triggers UI updates based on connection state
// - helps diagnose connection issues
function onConnectionStateChange() {
    if (!pc) {
        console.log('Connection state change called but pc is null');
        return;
    }

    const currentState = {
        connectionState: pc.connectionState,
        iceConnectionState: pc.iceConnectionState,
        iceGatheringState: pc.iceGatheringState,
        signalingState: pc.signalingState,
    };
    
    if (JSON.stringify(currentState) === JSON.stringify(lastConnectionState)) {
        return;
    }
    lastConnectionState = currentState;

    console.log('Connection state change:', currentState);

    switch (pc.connectionState) {
        case 'connected':
            console.log('Connection established, checking media tracks...');
            pc.getReceivers().forEach((receiver) => {
                console.log('Track:', receiver.track.kind, 'State:', receiver.track.readyState);
            });

            startConnectionMonitoring();
            startAudioStatsMonitoring();

            // Fetch and display the synth code
            console.log('Fetching synth code...');
            fetch('/synth-code', {
                headers: {
                    'X-Session-ID': sessionID
                }
            })
                .then(response => {
                    if (!response.ok) {
                        throw new Error(`Failed to fetch synth code: ${response.status}`);
                    }
                    return response.text();
                })
                .then(code => {
                    console.log('Code fetched, length:', code.length);
                    return typeCode(code);
                })
                .catch(error => console.error('Failed to fetch synth code:', error));
            break;

        case 'disconnected':
        case 'failed':
        case 'closed':
            console.log('Connection ended:', currentState);
            clearCode();
            break;
    }
}

// why we need ice connection monitoring:
// - tracks all candidate types for connectivity
// - helps identify network issues
// - provides detailed ICE state information
function onIceConnectionStateChange() {
    if (!pc) {
        console.log('ICE connection state change called but pc is null');
        return;
    }

    const now = new Date();
    console.log('[ICE] Connection state changed from', connectionState.lastState, 'to', pc.iceConnectionState, 'at', now);
    
    if (pc.iceConnectionState === 'checking') {
        console.log('[ICE] Connection checking - gathering candidates...');
        pc.getStats().then(stats => {
            let candidateTypes = new Set();
            let candidates = [];
            let transportStats = null;
            stats.forEach(stat => {
                if (stat.type === 'local-candidate') {
                    candidateTypes.add(stat.candidateType);
                    candidates.push({
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address,
                        port: stat.port,
                        priority: stat.priority,
                        url: stat.url,
                        relayProtocol: stat.relayProtocol
                    });
                } else if (stat.type === 'transport') {
                    transportStats = {
                        dtlsState: stat.dtlsState,
                        iceRole: stat.iceRole,
                        iceLocalUsernameFragment: stat.iceLocalUsernameFragment,
                        iceState: stat.iceState,
                        selectedCandidatePairId: stat.selectedCandidatePairId
                    };
                }
            });
            if (candidates.length > 0) {
                console.log('[ICE] Found candidates:', candidates);
                console.log('[ICE] Candidate types:', Array.from(candidateTypes));
                if (transportStats) {
                    console.log('[ICE] Transport stats:', transportStats);
                }
                if (!candidateTypes.has('relay')) {
                    console.warn('[ICE] No relay candidates found, this might indicate STUN server issues');
                }
            }
        });
    } else if (pc.iceConnectionState === 'disconnected') {
        connectionState.lastDisconnectTime = now;
        console.log('[ICE] Connection disconnected at', now);
        
        // Check if we should attempt immediate cleanup
        const timeSinceLastStateChange = connectionState.lastStateChangeTime ? 
            now - connectionState.lastStateChangeTime : 0;
        if (timeSinceLastStateChange < 5000) { // If state changed too quickly
            console.log('[ICE] Quick state change detected, may need cleanup');
        }

        // Log the last known transport stats
        pc.getStats().then(stats => {
            stats.forEach(stat => {
                if (stat.type === 'transport') {
                    console.log('[ICE] Last transport stats before disconnect:', {
                        dtlsState: stat.dtlsState,
                        iceRole: stat.iceRole,
                        iceState: stat.iceState,
                        selectedCandidatePairId: stat.selectedCandidatePairId,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent
                    });
                }
            });
        });
    } else if (pc.iceConnectionState === 'failed') {
        console.error('[ICE] Connection failed - checking stats...');
        pc.getStats().then(stats => {
            let diagnostics = {
                localCandidates: [],
                remoteCandidates: [],
                candidatePairs: [],
                transport: null,
                lastDisconnectTime: connectionState.lastDisconnectTime
            };
            
            stats.forEach(stat => {
                if (stat.type === 'local-candidate') {
                    diagnostics.localCandidates.push({
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address,
                        priority: stat.priority,
                        url: stat.url,
                        relayProtocol: stat.relayProtocol,
                        timestamp: stat.timestamp
                    });
                } else if (stat.type === 'remote-candidate') {
                    diagnostics.remoteCandidates.push({
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address,
                        timestamp: stat.timestamp
                    });
                } else if (stat.type === 'candidate-pair') {
                    diagnostics.candidatePairs.push({
                        state: stat.state,
                        nominated: stat.nominated,
                        bytesSent: stat.bytesSent,
                        bytesReceived: stat.bytesReceived,
                        timestamp: stat.timestamp,
                        localCandidateType: stat.localCandidateType,
                        remoteCandidateType: stat.remoteCandidateType,
                        priority: stat.priority,
                        writable: stat.writable,
                        requestsReceived: stat.requestsReceived,
                        requestsSent: stat.requestsSent,
                        responsesReceived: stat.responsesReceived,
                        responsesSent: stat.responsesSent,
                        consentRequestsSent: stat.consentRequestsSent
                    });
                } else if (stat.type === 'transport') {
                    diagnostics.transport = {
                        dtlsState: stat.dtlsState,
                        iceRole: stat.iceRole,
                        iceLocalUsernameFragment: stat.iceLocalUsernameFragment,
                        iceState: stat.iceState,
                        selectedCandidatePairId: stat.selectedCandidatePairId,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent
                    };
                }
            });
            console.error('[ICE] Connection diagnostics:', diagnostics);

            // Check for specific failure patterns
            const hasStunCandidates = diagnostics.localCandidates.some(c => c.type === 'relay');
            if (!hasStunCandidates) {
                console.error('[ICE] No STUN candidates found - possible STUN server connectivity issue');
            }

            const hasSuccessfulPairs = diagnostics.candidatePairs.some(p => p.state === 'succeeded');
            if (!hasSuccessfulPairs) {
                console.error('[ICE] No successful candidate pairs - possible connectivity issue');
            }

            const hasResponses = diagnostics.candidatePairs.some(p => p.responsesReceived > 0);
            if (!hasResponses) {
                console.error('[ICE] No responses received - possible firewall/NAT issue');
            }
        });
    }
    
    connectionState.lastState = pc.iceConnectionState;
    connectionState.lastStateChangeTime = now;
}

// why we need thorough cleanup:
// - prevents memory leaks
// - ensures proper resource release
// - maintains system stability
async function cleanupConnection() {
    console.log('[WebRTC] Starting connection cleanup');
    
    // Stop all monitoring intervals first
    [qualityMonitorInterval, audioStatsInterval, audioLevelsInterval, trackStatsInterval, codePollingInterval].forEach(interval => {
        if (interval) {
            clearInterval(interval);
            console.log('[CLEANUP] Cleared interval');
        }
    });

    // Reset all intervals
    qualityMonitorInterval = null;
    audioStatsInterval = null;
    audioLevelsInterval = null;
    trackStatsInterval = null;
    codePollingInterval = null;
    
    if (pc) {
        // Log final state before cleanup
        console.log('[WebRTC] Final connection state:', {
            connectionState: pc.connectionState,
            iceConnectionState: pc.iceConnectionState,
            iceGatheringState: pc.iceGatheringState,
            signalingState: pc.signalingState
        });
        
        // Send stop signal to server first
        try {
            await fetch('/stop', {
                method: 'POST',
                headers: {
                    'X-Session-ID': sessionID
                }
            });
            console.log('[WebRTC] Successfully sent stop signal to server');
        } catch (error) {
            console.error('[WebRTC] Error sending stop signal:', error);
        }
        
        // Stop all tracks
        pc.getReceivers().forEach(receiver => {
            if (receiver.track) {
                receiver.track.stop();
                console.log('[CLEANUP] Stopped track:', receiver.track.kind);
            }
        });
        
        // Close peer connection
        pc.close();
        console.log('[WebRTC] Closed peer connection');
        
        // Reset state
        connectionState = {
            lastState: null,
            lastStateChangeTime: null,
            successfulPairs: 0,
            gatheringComplete: false,
            lastDisconnectTime: null
        };
    }
    
    // Reset audio context and elements
    if (audioContext) {
        await audioContext.close();
        audioContext = null;
        console.log('[AUDIO] Closed audio context');
    }

    if (audioElement) {
        audioElement.pause();
        audioElement.srcObject = null;
        audioElement.remove();
        audioElement = null;
        console.log('[AUDIO] Removed audio element');
    }

    // Clear code display
    clearCode();
    
    // Reset all global variables
    pc = null;
    remoteDescriptionSet = false;
    pendingCandidates = [];
    lastConnectionState = null;
    
    console.log('[WebRTC] Cleanup complete');
}

// why we need code display functions:
// - provides visual feedback of synth code
// - handles code highlighting
// - manages code display state
function clearCode() {
    const codeDisplay = document.getElementById('codeDisplay');
    codeDisplay.textContent = '';
    codeDisplay.classList.remove('visible');
}

function typeCode(code) {
    const codeDisplay = document.getElementById('codeDisplay');
    codeDisplay.classList.add('language-supercollider');
    codeDisplay.classList.add('visible');
    
    // Set the code and trigger Prism highlighting
    codeDisplay.textContent = code;
    Prism.highlightElement(codeDisplay);
}

// why we need button event listeners:
// - handles user interaction
// - triggers WebRTC connection setup
// - manages connection state changes
document.addEventListener('DOMContentLoaded', () => {
    const synthButton = document.getElementById('synthButton');
    if (synthButton) {
        synthButton.addEventListener('click', handleSynthClick);
    }

    // Add HTMX event listener for server responses
    document.body.addEventListener('htmx:afterRequest', handleSynthResponse);
});

// why we need enhanced audio setup:
// - ensure proper audio pipeline initialization
// - detect browser audio issues early
// - monitor playback state changes
function setupAudioElement(track) {
    console.log('[AUDIO][SETUP] Setting up audio element for track:', {
        kind: track.kind,
        id: track.id,
        enabled: track.enabled,
        muted: track.muted,
        readyState: track.readyState,
        constraints: track.getConstraints(),
        settings: track.getSettings()
    });

    const stream = new MediaStream([track]);
    console.log('[AUDIO][SETUP] Created MediaStream:', {
        active: stream.active,
        id: stream.id
    });

    audioElement = new Audio();
    audioElement.autoplay = true;
    audioElement.controls = true;
    audioElement.style.display = 'none';
    audioElement.volume = 1.0; // Ensure volume is at maximum
    document.body.appendChild(audioElement);
    
    console.log('[AUDIO][SETUP] Created audio element:', {
        autoplay: audioElement.autoplay,
        controls: audioElement.controls,
        volume: audioElement.volume,
        muted: audioElement.muted,
        readyState: audioElement.readyState,
        networkState: audioElement.networkState,
        error: audioElement.error
    });

    audioElement.srcObject = stream;
    
    // Enhanced event listeners for audio element
    audioElement.addEventListener('loadstart', () => console.log('[AUDIO][STATE] Loading started'));
    audioElement.addEventListener('loadedmetadata', () => console.log('[AUDIO][STATE] Metadata loaded'));
    audioElement.addEventListener('loadeddata', () => console.log('[AUDIO][STATE] Data loaded'));
    audioElement.addEventListener('canplay', () => {
        console.log('[AUDIO][STATE] Can start playing');
        // Ensure audio context is resumed when we can play
        if (audioContext && audioContext.state === 'suspended') {
            audioContext.resume().then(() => {
                console.log('[AUDIO][CONTEXT] Resumed audio context');
            });
        }
    });
    audioElement.addEventListener('canplaythrough', () => console.log('[AUDIO][STATE] Can play through'));
    audioElement.addEventListener('play', () => console.log('[AUDIO][STATE] Playback started'));
    audioElement.addEventListener('pause', () => console.log('[AUDIO][STATE] Playback paused'));
    audioElement.addEventListener('ended', () => console.log('[AUDIO][STATE] Playback ended'));
    audioElement.addEventListener('error', (e) => {
        console.error('[AUDIO][ERROR] Playback error:', {
            error: e.target.error,
            networkState: audioElement.networkState,
            readyState: audioElement.readyState
        });
    });
    audioElement.addEventListener('stalled', () => {
        console.warn('[AUDIO][WARN] Playback stalled');
        // Try to recover from stall
        audioElement.load();
        audioElement.play().catch(err => {
            console.error('[AUDIO][ERROR] Failed to recover from stall:', err);
        });
    });
    audioElement.addEventListener('suspend', () => console.warn('[AUDIO][WARN] Playback suspended'));
    audioElement.addEventListener('waiting', () => {
        console.warn('[AUDIO][WARN] Waiting for data');
        // Check audio pipeline state
        if (audioContext) {
            console.log('[AUDIO][DEBUG] Audio context state:', audioContext.state);
            console.log('[AUDIO][DEBUG] Audio context time:', audioContext.currentTime);
        }
    });

    const playPromise = audioElement.play();
    if (playPromise !== undefined) {
        playPromise
            .then(() => {
                console.log('[AUDIO][PLAY] Playback started successfully');
                // Ensure audio context is running
                if (audioContext && audioContext.state === 'suspended') {
                    return audioContext.resume();
                }
            })
            .then(() => {
                if (audioContext) {
                    console.log('[AUDIO][CONTEXT] Audio context state after play:', audioContext.state);
                }
            })
            .catch(error => {
                console.error('[AUDIO][ERROR] Playback failed:', {
                    error: error,
                    name: error.name,
                    message: error.message,
                    audioState: audioElement.readyState,
                    networkState: audioElement.networkState,
                    paused: audioElement.paused,
                    currentTime: audioElement.currentTime,
                    contextState: audioContext ? audioContext.state : 'no context'
                });
            });
    }
}

// why we need reliable candidate sending:
// - ensures all candidates reach the server
// - handles transient network issues
// - maintains connection state
async function sendICECandidate(candidate) {
    const maxRetries = 3;
    let attempt = 0;
    
    while (attempt < maxRetries) {
        try {
            await fetch('/ice-candidate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Session-ID': sessionID
                },
                body: JSON.stringify({
                    candidate: btoa(JSON.stringify({
                        candidate: candidate.candidate,
                        sdpMid: candidate.sdpMid,
                        sdpMLineIndex: candidate.sdpMLineIndex,
                        usernameFragment: candidate.usernameFragment
                    }))
                })
            });
            iceProgress.trackAcknowledgement();
            return;
        } catch (err) {
            attempt++;
            if (attempt === maxRetries) throw err;
            await new Promise(resolve => setTimeout(resolve, 1000 * attempt));
        }
    }
}

// why we need unified stats monitoring:
// - combines connection quality and audio monitoring
// - provides comprehensive debugging data
// - reduces redundant stat collection
function startUnifiedMonitoring() {
    if (qualityMonitorInterval) {
        clearInterval(qualityMonitorInterval);
    }

    let lastBytesReceived = 0;
    let noDataCount = 0;
    const MAX_NO_DATA_COUNT = 10;

    qualityMonitorInterval = setInterval(async () => {
        if (!pc) return;

        try {
            const stats = await pc.getStats();
            const statsData = {
                audio: null,
                transport: null,
                candidatePair: null
            };

            stats.forEach(stat => {
                if (stat.type === 'inbound-rtp' && stat.kind === 'audio') {
                    statsData.audio = {
                        bytesReceived: stat.bytesReceived,
                        packetsReceived: stat.packetsReceived,
                        jitter: stat.jitter,
                        timestamp: stat.timestamp
                    };
                } else if (stat.type === 'transport') {
                    statsData.transport = {
                        dtlsState: stat.dtlsState,
                        selectedCandidatePairId: stat.selectedCandidatePairId,
                        bytesReceived: stat.bytesReceived
                    };
                } else if (stat.type === 'candidate-pair' && stat.state === 'succeeded') {
                    statsData.candidatePair = {
                        localCandidate: stat.localCandidateId,
                        remoteCandidate: stat.remoteCandidateId,
                        currentRoundTripTime: stat.currentRoundTripTime,
                        availableOutgoingBitrate: stat.availableOutgoingBitrate,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent
                    };
                }
            });

            // Log connection quality metrics
            if (statsData.candidatePair) {
                console.log('[STATS] Connection Quality:', {
                    rtt: statsData.candidatePair.currentRoundTripTime,
                    bitrate: statsData.candidatePair.availableOutgoingBitrate,
                    bytesReceived: statsData.candidatePair.bytesReceived,
                    bytesSent: statsData.candidatePair.bytesSent
                });
            }

            // Check audio flow
            if (statsData.audio) {
                const newBytes = statsData.audio.bytesReceived - lastBytesReceived;
                console.log('[AUDIO][FLOW]', {
                    newBytesReceived: newBytes,
                    totalBytesReceived: statsData.audio.bytesReceived,
                    packetsReceived: statsData.audio.packetsReceived,
                    jitter: statsData.audio.jitter,
                    dtlsState: statsData.transport?.dtlsState,
                    candidatePair: statsData.candidatePair ? 'active' : 'none'
                });

                if (newBytes === 0) {
                    noDataCount++;
                    if (noDataCount >= MAX_NO_DATA_COUNT) {
                        console.warn('[AUDIO][ALERT] No new audio data received for', noDataCount * 2, 'seconds');
                        console.log('[AUDIO][DEBUG] Connection state:', {
                            iceConnectionState: pc.iceConnectionState,
                            connectionState: pc.connectionState,
                            signalingState: pc.signalingState,
                            transport: statsData.transport,
                            candidatePair: statsData.candidatePair
                        });
                    }
                } else {
                    noDataCount = 0;
                }
                lastBytesReceived = statsData.audio.bytesReceived;
            } else {
                console.warn('[AUDIO][WARN] No inbound audio stats found');
            }
        } catch (error) {
            console.error('[STATS][ERROR] Failed to get stats:', error);
        }
    }, 2000);
}

// why we need comprehensive connection monitoring:
// - tracks dtls handshake progress
// - monitors ice connection states
// - logs detailed diagnostics on failure
function setupConnectionMonitoring(pc, monitorDTLS) {
    pc.oniceconnectionstatechange = () => {
        console.log('[ICE] Connection state changed:', pc.iceConnectionState);
        
        if (pc.iceConnectionState === 'checking') {
            console.log('[ICE] Connection checking - starting DTLS monitoring');
            monitorDTLS();
        } else if (pc.iceConnectionState === 'failed') {
            console.error('[ICE] Connection failed - gathering diagnostics...');
            console.log('[ICE] Local description:', pc.localDescription);
            console.log('[ICE] Remote description:', pc.remoteDescription);
            console.log('[ICE] ICE gathering state:', pc.iceGatheringState);
            console.log('[ICE] Signaling state:', pc.signalingState);
        }
    };

    pc.onicecandidate = (event) => {
        if (event.candidate) {
            const candidateObj = {
                candidate: event.candidate.candidate,
                sdpMid: event.candidate.sdpMid,
                sdpMLineIndex: event.candidate.sdpMLineIndex,
                usernameFragment: event.candidate.usernameFragment
            };

            // Parse candidate for logging
            const candidateStr = event.candidate.candidate;
            const parts = candidateStr.split(' ');
            const parsedCandidate = {
                foundation: parts[0].split(':')[1],
                component: parts[1],
                protocol: parts[2],
                priority: parts[3],
                address: parts[4],
                port: parts[5],
                type: parts[7],
                relAddr: parts[9] || undefined,
                relPort: parts[11] || undefined
            };

            console.log('[ICE] New candidate:', parsedCandidate);
            iceProgress.trackCandidate();
            
            if (!pc.remoteDescription) {
                console.log('[ICE] Queuing candidate until remote description is set');
                pendingCandidates.push(event.candidate);
                return;
            }

            sendICECandidate(event.candidate).catch(err => {
                console.error('[ICE] Failed to send candidate:', err);
                pendingCandidates.push(event.candidate);
            });
        }
    };
}

// Manual click handler (e.g., if not using HTMX for some flows)
async function handleSynthClick() {
    const button = document.querySelector('.connection-button');

    if (pc) {
        await cleanupConnection();
        button.textContent = 'Generate Synth';
        button.classList.remove('button-disconnect');
        return;
    }

    button.textContent = 'Connecting...';
    button.disabled = true;

    try {
        await setupWebRTC();
        button.textContent = 'Stop Synth';
        button.classList.add('button-disconnect');
        button.disabled = false;
    } catch (error) {
        console.error('Connection failed:', error);
        button.textContent = 'Error - Try Again';
        button.disabled = false;
    }
}

// why we need custom syntax highlighting:
// - improves code readability
// - helps identify different language elements
// - matches supercollider conventions
Prism.languages.supercollider = {
    'comment': {
        pattern: /(\/\/.*)|(\/\*[\s\S]*?\*\/)/,
        greedy: true
    },
    'string': {
        pattern: /"(?:\\.|[^\\"])*"/,
        greedy: true
    },
    'class-name': {
        pattern: /\b[A-Z]\w*\b/,
        greedy: true
    },
    'function': {
        pattern: /\b[a-z]\w*(?=\s*\()/,
        greedy: true
    },
    'keyword': /\b(?:arg|var|if|else|while|do|for|switch|case|return|nil|true|false|inf)\b/,
    'number': /\b-?(?:0x[\da-f]+|\d*\.?\d+(?:e[+-]?\d+)?)\b/i,
    'operator': /[-+*\/%=&|!<>^~?:]+/,
    'punctuation': /[{}[\];(),.:]/
};