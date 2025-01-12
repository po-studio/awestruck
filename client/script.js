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
        }, 10000); // Increased to 10 seconds for production

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

// why we need ice configuration:
// - enables peer discovery through our custom STUN server
// - allows direct peer connections where possible
// - falls back to host candidates if STUN fails
const ICE_CONFIG = {
    development: {
        iceServers: [
            {
                urls: [
                    window.STUN_SERVER ? `stun:${window.STUN_SERVER}` : "stun:localhost:3478"
                ]
            }
        ],
        iceCandidatePoolSize: 0,
        rtcpMuxPolicy: 'require',
        bundlePolicy: 'max-bundle',
        iceTransportPolicy: 'all',
        sdpSemantics: 'unified-plan'
    },
    production: {
        iceServers: [
            {
                urls: [
                    "stun:stun.awestruck.io:3478",
                ]
            }
        ],
        iceCandidatePoolSize: 1,
        rtcpMuxPolicy: 'require',
        bundlePolicy: 'max-bundle',
        iceTransportPolicy: 'all',
        sdpSemantics: 'unified-plan'
    }
};

// why we need ice configuration validation:
// - ensures our custom STUN server is properly configured
// - validates URL format
// - verifies required parameters are present
function validateIceConfig(config) {
    if (!config || !config.iceServers) {
        console.error('[ICE] Invalid ICE configuration');
        return false;
    }

    const hasStunServer = config.iceServers.some(server => {
        if (!server.urls) {
            console.error('[ICE] Missing URLs');
            return false;
        }

        // Validate URLs format and ensure they're STUN
        const validUrls = server.urls.every(url => {
            const isStun = url.startsWith('stun:');
            if (!isStun) {
                console.error('[ICE] URL must be STUN:', url);
                return false;
            }
            return true;
        });

        if (!validUrls) {
            console.error('[ICE] Invalid STUN URLs:', server.urls);
            return false;
        }

        return true;
    });

    if (!hasStunServer) {
        console.error('[ICE] No valid STUN server found in configuration');
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
        // Start connection as soon as we have a viable candidate
        if (pc.iceGatheringState !== 'new') {
            pc.getStats().then(stats => {
                let hasViableCandidate = false;
                stats.forEach(stat => {
                    if (stat.type === 'local-candidate' && 
                        (stat.candidateType === 'host' || stat.candidateType === 'srflx')) {
                        hasViableCandidate = true;
                    }
                });
                if (hasViableCandidate && !pc.remoteDescription) {
                    console.log('[ICE] Have viable candidate, proceeding with connection');
                    createAndSendOffer();
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
// - tracks STUN connectivity status
// - helps identify network issues
// - provides detailed ICE state information
function onIceConnectionStateChange() {
    if (!pc) {
        console.log('ICE connection state change called but pc is null');
        return;
    }

    console.log('ICE Connection State:', pc.iceConnectionState);
    
    if (pc.iceConnectionState === 'checking') {
        console.log('[ICE] Connection checking - gathering candidates...');
        pc.getStats().then(stats => {
            let candidateTypes = new Set();
            stats.forEach(stat => {
                if (stat.type === 'local-candidate') {
                    candidateTypes.add(stat.candidateType);
                    console.log('[ICE] Local candidate:', {
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address,
                        port: stat.port
                    });
                }
            });
            console.log('[ICE] Found candidate types:', Array.from(candidateTypes));
        });
    }

    if (pc.iceConnectionState === 'failed') {
        console.error('[ICE] Connection failed - checking stats...');
        pc.getStats().then(stats => {
            let diagnostics = {
                localCandidates: [],
                remoteCandidates: [],
                candidatePairs: []
            };
            
            stats.forEach(stat => {
                if (stat.type === 'local-candidate') {
                    diagnostics.localCandidates.push({
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address
                    });
                } else if (stat.type === 'remote-candidate') {
                    diagnostics.remoteCandidates.push({
                        type: stat.candidateType,
                        protocol: stat.protocol,
                        address: stat.address
                    });
                } else if (stat.type === 'candidate-pair') {
                    diagnostics.candidatePairs.push({
                        state: stat.state,
                        nominated: stat.nominated,
                        bytesSent: stat.bytesSent,
                        bytesReceived: stat.bytesReceived
                    });
                }
            });
            console.error('[ICE] Connection diagnostics:', diagnostics);
        });
    }
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

function startAudioStatsMonitoring() {
    if (audioStatsInterval) {
        clearInterval(audioStatsInterval);
    }

    audioStatsInterval = setInterval(() => {
        if (!pc) return;

        pc.getStats().then(stats => {
            stats.forEach(stat => {
                if (stat.type === 'inbound-rtp' && stat.kind === 'audio') {
                    console.log('[AUDIO] Detailed Stats:', {
                        packetsReceived: stat.packetsReceived,
                        packetsLost: stat.packetsLost,
                        jitter: stat.jitter,
                        bytesReceived: stat.bytesReceived,
                        timestamp: stat.timestamp,
                        audioLevel: stat.audioLevel,
                        totalSamplesReceived: stat.totalSamplesReceived,
                        concealedSamples: stat.concealedSamples,
                        silentConcealedSamples: stat.silentConcealedSamples,
                        codecId: stat.codecId
                    });
                } else if (stat.type === 'track' && stat.kind === 'audio') {
                    console.log('[AUDIO] Track Stats:', {
                        audioLevel: stat.audioLevel,
                        totalAudioEnergy: stat.totalAudioEnergy,
                        totalSamplesDuration: stat.totalSamplesDuration,
                        detached: stat.detached,
                        ended: stat.ended,
                        remoteSource: stat.remoteSource
                    });
                }
            });
        });
    }, 2000); // Reduced from 5s to 2s for faster feedback
}

// why we need proper cleanup:
// - ensures resources are released
// - stops all monitoring intervals
// - closes WebRTC connection properly
async function cleanupConnection() {
    console.log('Cleaning up connection...');

    // Clear all intervals first
    [qualityMonitorInterval, audioStatsInterval, audioLevelsInterval, 
     trackStatsInterval, codePollingInterval].forEach(interval => {
        if (interval) {
            clearInterval(interval);
            interval = null;
        }
    });

    // Stop audio context and clear element
    if (audioContext) {
        await audioContext.close();
        audioContext = null;
    }
    if (audioElement) {
        audioElement.pause();
        audioElement.remove();
        audioElement = null;
    }

    // Clear code display
    clearCode();

    // Close peer connection
    if (pc) {
        // Send stop signal to server
        try {
            await fetch('/stop', {
                method: 'POST',
                headers: {
                    'X-Session-ID': sessionID
                }
            });
        } catch (error) {
            console.error('Error sending stop signal:', error);
        }

        pc.close();
        pc = null;
    }

    console.log('Cleanup complete');
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

// why we need audio setup debugging:
// - tracks audio element creation and state
// - monitors stream attachment
// - verifies playback is working
function setupAudioElement(track) {
    console.log('[AUDIO] Setting up audio element for track:', {
        kind: track.kind,
        id: track.id,
        enabled: track.enabled,
        muted: track.muted,
        readyState: track.readyState
    });

    const stream = new MediaStream([track]);
    console.log('[AUDIO] Created MediaStream:', {
        active: stream.active,
        id: stream.id
    });

    audioElement = new Audio();
    audioElement.autoplay = true;
    audioElement.controls = true;
    audioElement.style.display = 'none';
    document.body.appendChild(audioElement);
    
    console.log('[AUDIO] Created audio element:', {
        autoplay: audioElement.autoplay,
        controls: audioElement.controls,
        volume: audioElement.volume,
        muted: audioElement.muted
    });

    audioElement.srcObject = stream;
    
    // Add event listeners for audio element
    audioElement.addEventListener('play', () => console.log('[AUDIO] Audio element started playing'));
    audioElement.addEventListener('pause', () => console.log('[AUDIO] Audio element paused'));
    audioElement.addEventListener('ended', () => console.log('[AUDIO] Audio element ended'));
    audioElement.addEventListener('error', (e) => console.error('[AUDIO] Audio element error:', e));
    
    // Set up audio processing and monitoring
    try {
        audioContext = new (window.AudioContext || window.webkitAudioContext)();
        console.log('[AUDIO] Created AudioContext:', {
            sampleRate: audioContext.sampleRate,
            state: audioContext.state,
            baseLatency: audioContext.baseLatency,
            outputLatency: audioContext.outputLatency
        });
        
        const source = audioContext.createMediaStreamSource(stream);
        const analyser = audioContext.createAnalyser();
        analyser.fftSize = 2048;
        source.connect(analyser);
        
        // Create data array for monitoring
        const dataArray = new Float32Array(analyser.frequencyBinCount);
        
        // Monitor audio levels
        audioLevelsInterval = setInterval(() => {
            if (audioContext && audioContext.state === 'running') {
                analyser.getFloatTimeDomainData(dataArray);
                let maxLevel = 0;
                for (let i = 0; i < dataArray.length; i++) {
                    maxLevel = Math.max(maxLevel, Math.abs(dataArray[i]));
                }
                console.log('[AUDIO] Current level:', maxLevel.toFixed(4));
            }
        }, 500); // Monitor every 500ms
    } catch (error) {
        console.error('[AUDIO] Failed to setup audio processing:', error);
    }

    const playPromise = audioElement.play();
    if (playPromise !== undefined) {
        playPromise
            .then(() => console.log('[AUDIO] Audio playback started successfully'))
            .catch(error => console.error('[AUDIO] Audio playback failed:', error));
    }
}