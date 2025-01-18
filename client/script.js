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

// Generate or retrieve session ID once
function getSessionID() {
  let sessionID = sessionStorage.getItem('sessionID');
  if (!sessionID) {
      sessionID = `sid_${Math.random().toString(36).slice(2, 11)}_${Date.now()}`;
      sessionStorage.setItem('sessionID', sessionID);
  }
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
      const isProduction = window.location.hostname !== 'localhost';
      const config = isProduction ? ICE_CONFIG.production : ICE_CONFIG.development;
      
      console.log('Using WebRTC config:', config);

      await setupWebRTC(config);

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
        }, 30000); // Increased to 30 seconds to match server gathering timeout

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

// why we need both stun and turn servers:
// - stun for basic nat traversal
// - turn as fallback for symmetric nat
// - improves connection reliability
window.TURN_SERVERS = window.location.hostname === 'localhost'
  ? ['localhost:3478']
  : ['turn.awestruck.io:3478'];

// why we use a static credential:
// - simplifies initial implementation
// - can be replaced with dynamic auth later
// - allows for testing and development
const TURN_CREDENTIAL = 'awestruck-turn-static-auth-key';

// why we need ice configuration:
// - enables discovery through our STUN/TURN server
// - allows host candidates for container communication
// - ensures consistent behavior across environments
const ICE_CONFIG = {
    development: {
        iceServers: [
            {
                urls: window.TURN_SERVERS.map(server => [
                    `stun:${server}`,
                    `turn:${server}`
                ]).flat(),
                username: 'default',
                credential: TURN_CREDENTIAL
            }
        ],
        iceCandidatePoolSize: 1,
        rtcpMuxPolicy: 'require',
        bundlePolicy: 'max-bundle',
        iceTransportPolicy: 'all'
    },
    production: {
        iceServers: [
            {
                urls: window.TURN_SERVERS.map(server => [
                    `stun:${server}`,
                    `turn:${server}`
                ]).flat(),
                username: 'default',
                credential: TURN_CREDENTIAL
            },
            {
                urls: [
                    "stun:stun.l.google.com:19302",
                    "stun:stun1.l.google.com:19302"
                ]
            }
        ],
        iceCandidatePoolSize: 1,
        rtcpMuxPolicy: 'require',
        bundlePolicy: 'max-bundle',
        iceTransportPolicy: 'all'
    }
};

// why we need flexible ice validation:
// - allows both STUN and TURN candidates
// - supports container-to-container communication
// - maintains logging for monitoring
function validateIceConfig(config) {
    if (!config || !config.iceServers) {
        console.error('[ICE] Invalid ICE configuration');
        return false;
    }

    const hasValidServer = config.iceServers.some(server => {
        if (!server.urls) {
            console.error('[ICE] Missing URLs');
            return false;
        }

        // Log all URL types for monitoring
        server.urls.forEach(url => {
            if (url.startsWith('stun:')) {
                console.log('[ICE] Found STUN URL:', url);
            } else if (url.startsWith('turn:')) {
                console.log('[ICE] Found TURN URL:', url);
                if (!server.username || !server.credential) {
                    console.error('[ICE] TURN server missing credentials');
                    return false;
                }
            }
        });

        return true;
    });

    if (!hasValidServer) {
        console.error('[ICE] No valid server configuration found');
        return false;
    }

    return true;
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
        const isProduction = window.location.hostname !== 'localhost';
        const config = isProduction ? ICE_CONFIG.production : ICE_CONFIG.development;
        
        console.log('Using WebRTC config:', config);

        await setupWebRTC(config);

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

// why we need webrtc setup:
// - initializes peer connection with STUN configuration
// - sets up media tracks and data channels
// - handles connection state changes
async function setupWebRTC(config) {
    if (!validateIceConfig(config)) {
        throw new Error('Invalid ICE configuration');
    }

    console.log('[ICE] Using STUN servers:', config.iceServers.map(server => server.urls));

    pc = new RTCPeerConnection(config);
    console.log('[WebRTC] Created peer connection with config:', config);

    // Add early media handling
    let audioTransceiver = pc.addTransceiver('audio', {
        direction: 'recvonly',
        streams: [new MediaStream()]
    });
    console.log('[WebRTC] Added audio transceiver:', audioTransceiver);

    // Optimize ICE gathering
    pc.onicegatheringstatechange = () => {
        console.log('[ICE] Gathering state:', pc.iceGatheringState);
        
        // Monitor candidate gathering progress
        if (pc.iceGatheringState !== 'new') {
            pc.getStats().then(stats => {
                // why we track different stats types:
                // - local-candidate: our STUN-discovered endpoints
                // - remote-candidate: server's endpoints
                // - candidate-pair: actual connections being attempted
                // - transport: overall ICE transport status
                let statsReport = {
                    localCandidates: [],
                    remoteCandidates: [],
                    candidatePairs: [],
                    transport: null
                };
                
                stats.forEach(stat => {
                    switch(stat.type) {
                        case 'local-candidate':
                            statsReport.localCandidates.push({
                                type: stat.candidateType,
                                protocol: stat.protocol,
                                address: stat.address,
                                port: stat.port
                            });
                            break;
                        case 'remote-candidate':
                            statsReport.remoteCandidates.push({
                                type: stat.candidateType,
                                protocol: stat.protocol,
                                address: stat.address,
                                port: stat.port
                            });
                            break;
                        case 'candidate-pair':
                            if (stat.state === 'succeeded') {
                                statsReport.candidatePairs.push({
                                    state: stat.state,
                                    localCandidateId: stat.localCandidateId,
                                    remoteCandidateId: stat.remoteCandidateId,
                                    priority: stat.priority,
                                    nominated: stat.nominated,
                                    writable: stat.writable
                                });
                            }
                            break;
                        case 'transport':
                            statsReport.transport = {
                                bytesReceived: stat.bytesReceived,
                                bytesSent: stat.bytesSent,
                                dtlsState: stat.dtlsState,
                                iceState: stat.iceState,
                                selectedCandidatePairId: stat.selectedCandidatePairId
                            };
                            break;
                    }
                });

                // Log only if we have STUN candidates or successful pairs
                if (statsReport.localCandidates.length > 0 || statsReport.candidatePairs.length > 0) {
                    console.log('[ICE] Connection stats:', statsReport);
                }
            });
        }
    };

    // Set up connection state monitoring
    pc.onconnectionstatechange = onConnectionStateChange;
    pc.oniceconnectionstatechange = onIceConnectionStateChange;
    
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
                
                const source = audioContext.createMediaStreamSource(new MediaStream([event.track]));
                const analyser = audioContext.createAnalyser();
                analyser.fftSize = 2048;
                source.connect(analyser);
                
                // Monitor audio levels using analyser
                const dataArray = new Float32Array(analyser.frequencyBinCount);
                const checkAudioLevels = () => {
                    if (audioContext && audioContext.state === 'running') {
                        analyser.getFloatTimeDomainData(dataArray);
                        let maxLevel = 0;
                        for (let i = 0; i < dataArray.length; i++) {
                            maxLevel = Math.max(maxLevel, Math.abs(dataArray[i]));
                        }
                        console.log('[AUDIO] Current level:', maxLevel.toFixed(4));
                    }
                };
                audioLevelsInterval = setInterval(checkAudioLevels, 500); // Reduced from 1s to 500ms for more responsive level monitoring
            } catch (error) {
                console.error('[AUDIO] Failed to setup audio processing:', error);
            }
        }
    };

    pc.onicecandidate = (event) => {
        if (event.candidate) {
            // in production, only allow srflx candidates
            const isProduction = window.location.hostname !== 'localhost';
            if (isProduction) {
                const candidateObj = event.candidate.toJSON();
                if (candidateObj.type !== 'srflx') {
                    console.log('[ICE] Filtering out non-srflx candidate in production:', candidateObj);
                    return;
                }
            }

            iceProgress.trackCandidate();
            console.log('[ICE] New candidate:', event.candidate);
            
            // Only send candidates after remote description is set
            if (!pc.remoteDescription) {
                console.log('[ICE] Waiting for remote description before sending candidate');
                pendingCandidates.push(event.candidate);
                return;
            }

            fetch('/ice-candidate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Session-ID': sessionID
                },
                body: JSON.stringify({
                    candidate: btoa(JSON.stringify({
                        candidate: event.candidate.candidate,
                        sdpMid: event.candidate.sdpMid,
                        sdpMLineIndex: event.candidate.sdpMLineIndex,
                        usernameFragment: event.candidate.usernameFragment
                    }))
                })
            }).then(() => {
                iceProgress.trackAcknowledgement();
            }).catch(err => {
                console.error('[ICE] Failed to send candidate:', err);
            });
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

// why we need connection monitoring:
// - tracks WebRTC connection quality
// - monitors audio stats for debugging
// - helps identify performance issues
function startConnectionMonitoring() {
    if (qualityMonitorInterval) {
        clearInterval(qualityMonitorInterval);
    }

    qualityMonitorInterval = setInterval(() => {
        if (!pc) return;

        pc.getStats().then(stats => {
            stats.forEach(stat => {
                if (stat.type === 'candidate-pair' && stat.state === 'succeeded') {
                    console.log('[STATS] Connection Quality:', {
                        currentRoundTripTime: stat.currentRoundTripTime,
                        availableOutgoingBitrate: stat.availableOutgoingBitrate,
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent
                    });
                }
            });
        });
    }, 2000); // Reduced from 5s to 2s for faster feedback
}

// why we need enhanced audio monitoring:
// - track audio pipeline health
// - detect playback issues early
// - monitor network impact on audio
function startAudioStatsMonitoring() {
    if (audioStatsInterval) {
        clearInterval(audioStatsInterval);
    }

    audioStatsInterval = setInterval(() => {
        if (!pc) return;

        pc.getStats().then(stats => {
            let audioStats = {
                inbound: {},
                track: {},
                transport: {},
                candidatePair: {}
            };

            stats.forEach(stat => {
                if (stat.type === 'inbound-rtp' && stat.kind === 'audio') {
                    audioStats.inbound = {
                        packetsReceived: stat.packetsReceived,
                        packetsLost: stat.packetsLost,
                        jitter: stat.jitter,
                        bytesReceived: stat.bytesReceived,
                        timestamp: stat.timestamp,
                        audioLevel: stat.audioLevel,
                        totalSamplesReceived: stat.totalSamplesReceived,
                        concealedSamples: stat.concealedSamples,
                        silentConcealedSamples: stat.silentConcealedSamples,
                        codecId: stat.codecId,
                        fecPacketsReceived: stat.fecPacketsReceived,
                        fecPacketsDiscarded: stat.fecPacketsDiscarded,
                        packetsDiscarded: stat.packetsDiscarded,
                        nackCount: stat.nackCount
                    };
                    
                    // Calculate packet loss percentage
                    if (stat.packetsReceived > 0) {
                        audioStats.inbound.packetLossPercent = 
                            (stat.packetsLost / (stat.packetsReceived + stat.packetsLost)) * 100;
                    }
                    
                    console.log('[AUDIO][STATS] Inbound RTP:', audioStats.inbound);
                } else if (stat.type === 'track' && stat.kind === 'audio') {
                    audioStats.track = {
                        audioLevel: stat.audioLevel,
                        totalAudioEnergy: stat.totalAudioEnergy,
                        totalSamplesDuration: stat.totalSamplesDuration,
                        detached: stat.detached,
                        ended: stat.ended,
                        remoteSource: stat.remoteSource,
                        jitterBufferDelay: stat.jitterBufferDelay,
                        jitterBufferEmittedCount: stat.jitterBufferEmittedCount
                    };
                    
                    // Calculate effective jitter buffer in ms
                    if (stat.jitterBufferEmittedCount > 0) {
                        audioStats.track.avgJitterBufferMs = 
                            (stat.jitterBufferDelay / stat.jitterBufferEmittedCount) * 1000;
                    }
                    
                    console.log('[AUDIO][STATS] Media Track:', audioStats.track);
                } else if (stat.type === 'transport') {
                    audioStats.transport = {
                        bytesReceived: stat.bytesReceived,
                        bytesSent: stat.bytesSent,
                        dtlsState: stat.dtlsState,
                        selectedCandidatePairId: stat.selectedCandidatePairId,
                        iceRole: stat.iceRole,
                        iceState: stat.iceState
                    };
                    console.log('[AUDIO][STATS] Transport:', audioStats.transport);
                } else if (stat.type === 'candidate-pair' && stat.selected) {
                    audioStats.candidatePair = {
                        availableOutgoingBitrate: stat.availableOutgoingBitrate,
                        currentRoundTripTime: stat.currentRoundTripTime,
                        localCandidateType: stat.localCandidateType,
                        remoteCandidateType: stat.remoteCandidateType,
                        state: stat.state,
                        totalRoundTripTime: stat.totalRoundTripTime,
                        responsesReceived: stat.responsesReceived
                    };
                    console.log('[AUDIO][STATS] Active Candidate Pair:', audioStats.candidatePair);
                }
            });

            // Log critical metrics that might affect audio quality
            if (audioStats.inbound.packetLossPercent > 5) {
                console.warn('[AUDIO][QUALITY] High packet loss detected:', 
                    audioStats.inbound.packetLossPercent.toFixed(2) + '%');
            }
            if (audioStats.inbound.jitter > 50) {
                console.warn('[AUDIO][QUALITY] High jitter detected:', 
                    audioStats.inbound.jitter.toFixed(2) + 'ms');
            }
            if (audioStats.track.avgJitterBufferMs > 250) {
                console.warn('[AUDIO][QUALITY] Large jitter buffer:', 
                    audioStats.track.avgJitterBufferMs.toFixed(2) + 'ms');
            }
        });
    }, 2000);
}

// why we need proper cleanup:
// - ensures resources are released
// - stops all monitoring intervals
// - closes WebRTC connection properly
async function cleanupConnection() {
    console.log('[WebRTC] Starting connection cleanup');
    
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
            }
        });
        
        // Close peer connection
        pc.close();
        
        // Reset state
        connectionState = {
            lastState: null,
            lastStateChangeTime: null,
            successfulPairs: 0,
            gatheringComplete: false,
            lastDisconnectTime: null
        };
    }
    
    // Clear all intervals
    [qualityMonitorInterval, audioStatsInterval, audioLevelsInterval].forEach(interval => {
        if (interval) {
            clearInterval(interval);
        }
    });
    
    // Reset audio context
    if (audioContext) {
        await audioContext.close();
        audioContext = null;
    }

    // Clear code display
    clearCode();
    
    pc = null;
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
    
    // Set up audio processing and monitoring
    try {
        audioContext = new (window.AudioContext || window.webkitAudioContext)();
        console.log('[AUDIO][CONTEXT] Created AudioContext:', {
            sampleRate: audioContext.sampleRate,
            state: audioContext.state,
            baseLatency: audioContext.baseLatency,
            outputLatency: audioContext.outputLatency,
            destination: {
                maxChannelCount: audioContext.destination.maxChannelCount,
                numberOfInputs: audioContext.destination.numberOfInputs,
                numberOfOutputs: audioContext.destination.numberOfOutputs
            }
        });
        
        const source = audioContext.createMediaStreamSource(stream);
        const analyser = audioContext.createAnalyser();
        analyser.fftSize = 2048;
        analyser.smoothingTimeConstant = 0.8;
        
        // Connect source to both analyser and destination
        source.connect(analyser);
        source.connect(audioContext.destination);
        
        // Enhanced audio level monitoring
        const dataArray = new Float32Array(analyser.frequencyBinCount);
        let silentFrames = 0;
        const MAX_SILENT_FRAMES = 50; // 25 seconds at 500ms interval
        
        audioLevelsInterval = setInterval(() => {
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
                    contextTime: audioContext.currentTime
                });

                // Detect prolonged silence
                if (maxLevel < 0.01) {
                    silentFrames++;
                    if (silentFrames >= MAX_SILENT_FRAMES) {
                        console.warn('[AUDIO][WARN] Prolonged silence detected');
                        // Try to recover audio pipeline
                        audioContext.resume().then(() => {
                            console.log('[AUDIO][RECOVERY] Resumed audio context after silence');
                        });
                        silentFrames = 0;
                    }
                } else {
                    silentFrames = 0;
                }
            }
        }, 500);
    } catch (error) {
        console.error('[AUDIO][ERROR] Failed to setup audio processing:', error);
    }

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