// Generate or retrieve session ID once
function getSessionID() {
  let sessionID = sessionStorage.getItem('sessionID');
  if (!sessionID) {
      sessionID = `sid_${Math.random().toString(36).substr(2, 9)}`;
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
      const turnResponse = await fetch('/turn-credentials', {
          headers: { 'X-Session-ID': sessionID }
      });
      const fetchedTurnConfig = await turnResponse.json();

      await setupWebRTC(fetchedTurnConfig);

      button.textContent = 'Stop Synth';
      button.classList.add('button-disconnect');
      button.disabled = false;
  } catch (error) {
      console.error('Connection failed:', error);
      button.textContent = 'Error - Try Again';
      button.disabled = false;
  }
}

async function setupWebRTC(config) {
  if (!validateTurnConfig(config)) {
      throw new Error('Invalid TURN configuration');
  }

  console.log('Starting WebRTC setup with config:', {
      iceTransportPolicy: config.iceTransportPolicy,
      iceServers: config.iceServers.map(s => ({
          urls: s.urls,
          hasCredentials: !!(s.username && s.credential)
      }))
  });

  pc = new RTCPeerConnection(config);

  pc.onconnectionstatechange = onConnectionStateChange;
  // pc.onicecandidate = handleICECandidate;
  pc.oniceconnectionstatechange = onIceConnectionStateChange;
  pc.onicegatheringstatechange = onIceGatheringStateChange;
  pc.ontrack = handleTrack;

  pc.addTransceiver('audio', { direction: 'recvonly' });

  const checkIceCandidates = monitorIceCandidates(pc);
  pc.onicecandidate = (event) => {
      handleICECandidate(event);  // Your existing handler
      if (event.candidate) {
          checkIceCandidates(event.candidate);  // Additional monitoring
      }
  };

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);

  const browserOffer = {
      sdp: btoa(JSON.stringify({
          type: offer.type,
          sdp: offer.sdp
      })),
      type: offer.type,
      iceServers: config.iceServers
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
      throw new Error(`Server returned ${response.status}`);
  }

  const answer = await response.json();
  await pc.setRemoteDescription(new RTCSessionDescription(answer));

  // (Optional) If you need to send any ICE candidates stored before remote desc:
  // pendingCandidates.forEach(candidate => sendIceCandidate(candidate));
  // pendingCandidates = [];
}

function handleTrack(event) {
  if (!audioElement) {
      audioElement = new Audio();
      audioElement.autoplay = true;
  }
  audioElement.srcObject = event.streams[0];
}

// Store pending ICE candidates to send when remote desc is ready
let pendingCandidates = [];

async function handleICECandidate(event) {
  if (!event.candidate) return;
  
  const candidateStr = event.candidate.candidate;
  const parts = candidateStr.split(' ');
  const type = parts[7];
  const isProduction = window.location.hostname !== 'localhost';

  console.log("ICE candidate details:", {
    type,
    protocol: parts[2],
    ip: parts[4],
    port: parts[5],
    isFiltered: isProduction && type !== 'relay',
    fullCandidate: candidateStr,
  });
  
  if (isProduction && type !== 'relay') {
    console.warn('Filtered non-relay candidate');
    return;
  }

  const candidateObj = {
    candidate: {
      candidate: event.candidate.candidate,
      sdpMid: event.candidate.sdpMid,
      sdpMLineIndex: event.candidate.sdpMLineIndex,
      usernameFragment: event.candidate.usernameFragment
    }
  };

  try {
    const response = await fetch('/ice-candidate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID
      },
      body: JSON.stringify(candidateObj)
    });
    if (!response.ok) {
      console.warn(`Failed to send ICE candidate: ${response.status}`);
    }
  } catch (error) {
    console.warn('Failed to send ICE candidate:', error);
  }
}

function onConnectionStateChange() {
  if (!pc) {
    console.log('Connection state change called but pc is null');
    return;
  }

  // Prevent recursive calls by checking if state has actually changed
  const currentState = {
    connectionState: pc.connectionState,
    iceConnectionState: pc.iceConnectionState,
    iceGatheringState: pc.iceGatheringState,
    signalingState: pc.signalingState,
  };
  
  // Store previous state in a closure or instance variable
  if (JSON.stringify(currentState) === JSON.stringify(this.lastState)) {
    return;
  }
  this.lastState = currentState;

  console.log('Connection state change:', currentState);

  switch (pc.connectionState) {
    case 'connected':
      console.log('Connection established, checking media tracks...');
      pc.getReceivers().forEach((receiver) => {
        console.log('Track:', receiver.track.kind, 'State:', receiver.track.readyState);
      });

      startConnectionMonitoring();
      startAudioStatsMonitoring();

      // updateToggleButton({ text: 'Disconnect', disconnectStyle: true, disabled: false });
      
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

async function cleanupConnection() {
  const sessionInfo = {
      sessionId: sessionID,
      lastConnectionState: pc ? pc.connectionState : 'none',
      lastIceState: pc ? pc.iceConnectionState : 'none',
      timeStamp: new Date().toISOString()
  };

  console.log('Cleaning up session:', sessionInfo);

  // Clear monitoring intervals
  clearInterval(qualityMonitorInterval);
  if (codePollingInterval) {
      clearInterval(codePollingInterval);
      codePollingInterval = null;
  }
  // Clear ICE timeout if it exists
  if (iceCheckingTimeout) {
      clearTimeout(iceCheckingTimeout);
      iceCheckingTimeout = null;
  }

  // Remove audio element
  if (audioElement) {
      audioElement.srcObject = null;
      audioElement.remove();
      audioElement = null;
  }

  // Close audio context
  if (audioContext) {
      await audioContext.close();
      audioContext = null;
      console.log('Audio context closed successfully');
  }

  // Close peer connection
  if (pc) {
      logLastKnownGoodConnection();
      pc.close();
      pc = null;
  }

  // Tell server to stop
  try {
      const response = await fetch('/stop', {
          method: 'POST',
          headers: {
              'Content-Type': 'application/json',
              'X-Session-ID': sessionID
          }
      });
      if (!response.ok) {
          console.error('Failed to stop server session:', response.status);
      }
  } catch (error) {
      console.error('Error stopping server session:', error);
  }

  // Fade out and clear code
  const codeDisplay = document.getElementById('codeDisplay');
  codeDisplay.style.transition = 'opacity 0.3s ease-out';
  codeDisplay.style.opacity = '0';
  setTimeout(() => {
      clearCode();
  }, 300);

  console.log('Session cleanup completed:', {
      ...sessionInfo,
      intervalsCleared: true,
      peerConnectionClosed: true
  });
}

async function typeCode(code) {
    const codeDisplay = document.getElementById('codeDisplay');
    if (!codeDisplay) return;

    clearCode();
    codeDisplay.classList.add('visible');
    
    // Ensure language class is added before content
    codeDisplay.classList.add('language-supercollider');
    
    const chars = code.split('');
    let currentText = '';

    for (const char of chars) {
        currentText += char;
        codeDisplay.textContent = currentText;
        if (window.Prism) {
            Prism.highlightElement(codeDisplay);
        }
        await new Promise(resolve => setTimeout(resolve, Math.random() * 4 + 1));
    }
}

function clearCode() {
  const codeDisplay = document.getElementById('codeDisplay');
  if (codeDisplay) {
      codeDisplay.textContent = '';
      codeDisplay.style.opacity = '1';
      codeDisplay.style.transition = '';
      codeDisplay.classList.remove('visible', 'language-supercollider');
  }
}

// Connection quality monitoring
function startConnectionMonitoring() {
  clearInterval(qualityMonitorInterval);
  qualityMonitorInterval = setInterval(() => {
      if (!pc) {
          clearInterval(qualityMonitorInterval);
          return;
      }
      pc.getStats().then(stats => {
          stats.forEach(report => {
              switch (report.type) {
                  case 'candidate-pair':
                      if (report.state === 'succeeded') {
                          console.log('Connection Quality:', {
                              currentRoundTripTime: report.currentRoundTripTime,
                              availableOutgoingBitrate: report.availableOutgoingBitrate,
                              bytesReceived: report.bytesReceived,
                              protocol: report.protocol,
                              relayProtocol: report.relayProtocol,
                              localCandidateType: report.localCandidateType,
                              remoteCandidateType: report.remoteCandidateType
                          });
                      }
                      break;
                  case 'inbound-rtp':
                      if (report.kind === 'audio') {
                          console.log('Audio Stats:', {
                              packetsReceived: report.packetsReceived,
                              bytesReceived: report.bytesReceived,
                              packetsLost: report.packetsLost,
                              jitter: report.jitter
                          });
                      }
                      break;
              }
          });
      });
  }, 1000);
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
      const config = isProduction ? TURN_CONFIG.production : TURN_CONFIG.development;
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

// Separate TURN configs for local vs. production
const TURN_CONFIG = {
  development: {
      iceServers: [
          {
              urls: [
                "turn:localhost:3478?transport=udp",
                "turn:localhost:3478?transport=tcp"
              ],
              username: "awestruck",
              credential: "password",
              credentialType: "password",
              realm: "localhost"
          }
      ],
      iceTransportPolicy: 'all',
      iceCandidatePoolSize: 1
  },
  production: {
      iceServers: [
          {
              urls: [
                "turn:turn.awestruck.io:3478?transport=udp",
                "turn:turn.awestruck.io:3478?transport=tcp"
              ],
              username: "awestruck",
              credential: "password",
              credentialType: "password",
              realm: "awestruck.io"
          }
      ],
      iceTransportPolicy: 'relay',
      iceCandidatePoolSize: 1
  }
};

function validateTurnConfig(config) {
  if (!config || !config.iceServers) {
      console.error('Invalid ICE configuration');
      return false;
  }

  const hasTurnServer = config.iceServers.some(server => {
      return server.urls.some(url => 
          url.startsWith('turn:') || url.startsWith('turns:')
      );
  });

  if (!hasTurnServer) {
      console.error('No TURN server found in configuration');
      return false;
  }

  console.log('ICE Configuration validated:', config);
  return true;
}

function onIceConnectionStateChange() {
  console.log('ICE Connection State:', pc.iceConnectionState);

  pc.onconnectionstatechange = () => {
    console.log('[WebRTC] Connection state changed:', {
      connectionState: pc.connectionState,
      iceConnectionState: pc.iceConnectionState,
      signalingState: pc.signalingState
    });
    
    if (pc.connectionState === 'failed') {
      pc.getStats().then(stats => {
        const diagnostics = {
          timestamp: new Date().toISOString(),
          candidates: [],
          transportStats: []
        };
        
        stats.forEach(stat => {
          if (stat.type === 'candidate-pair') {
            diagnostics.candidates.push(stat);
          }
          if (stat.type === 'transport') {
            diagnostics.transportStats.push(stat);
          }
        });
        
        console.error('[WebRTC] Connection failed diagnostics:', diagnostics);
      });
    }
  };
  
  if (pc.iceConnectionState === 'checking' || pc.iceConnectionState === 'connected') {
      monitorTurnConnectivity(pc);
  }

  const timestamp = new Date().toISOString();
  const diagnosticInfo = {
      timestamp,
      iceState: pc.iceConnectionState,
      connectionState: pc.connectionState,
      signalingState: pc.signalingState,
      sessionId: sessionID
  };

  console.log('[ICE] State change diagnostic info:', diagnosticInfo);

  if (['checking', 'failed'].includes(pc.iceConnectionState)) {
      pc.getStats().then(stats => {
          const candidatePairs = [];
          stats.forEach(stat => {
              if (stat.type === 'candidate-pair') {
                  candidatePairs.push({
                      state: stat.state,
                      nominated: stat.nominated,
                      priority: stat.priority
                  });
              }
          });
          console.log('[ICE] Candidate pairs during', pc.iceConnectionState, ':', candidatePairs);
      });
  }

  if (iceCheckingTimeout) {
      clearTimeout(iceCheckingTimeout);
      iceCheckingTimeout = null;
  }

  if (pc.iceConnectionState === 'checking') {
      iceCheckingTimeout = setTimeout(() => {
          if (pc && pc.iceConnectionState === 'checking') {
              console.warn('ICE checking timeout - forcing cleanup');
              cleanupConnection();
          }
      }, 30000);
  }
  if (pc.iceConnectionState === 'failed') {
      console.error('ICE Connection failed - gathering diagnostic information');
      logLastKnownGoodConnection();
      if (pc) {
          pc.getStats().then(stats => {
              console.log('Final ICE stats before failure:', stats);
          });
      }
      const isProduction = window.location.hostname !== 'localhost';
      if (!isProduction) {
          console.log('Development environment - immediate cleanup on failure');
          cleanupConnection();
          return;
      }
      if (pc) {
          pc.restartIce();
          setTimeout(() => {
              if (pc && pc.iceConnectionState === 'failed') {
                  cleanupConnection();
              }
          }, 3000);
      }
  }
}

function onIceGatheringStateChange() {
  if (!pc) return;
  console.log(`ICE gathering state: ${pc.iceGatheringState}`);

  const diagnosticInfo = {
      timestamp: new Date().toISOString(),
      gatheringState: pc.iceGatheringState,
      connectionState: pc.connectionState,
      iceConnectionState: pc.iceConnectionState,
      signalingState: pc.signalingState
  };

  console.log('[ICE] Gathering state diagnostic info:', diagnosticInfo);

  if (pc.iceGatheringState === 'complete') {
      console.log('Final SDP with ICE candidates:', pc.localDescription.sdp);
      pc.getStats().then(stats => {
          const localCandidates = [];
          stats.forEach(stat => {
              if (stat.type === 'local-candidate') {
                  localCandidates.push({
                      type: stat.candidateType,
                      protocol: stat.protocol,
                      address: stat.address,
                      port: stat.port,
                      priority: stat.priority
                  });
              }
          });
          console.log('[ICE] Gathered local candidates:', localCandidates);
      });
  }
}

function startAudioStatsMonitoring() {
  clearInterval(audioStatsInterval);
  audioStatsInterval = setInterval(() => {
    if (!pc) {
      clearInterval(audioStatsInterval);
      return;
    }
    pc.getStats().then((stats) => {
      stats.forEach((report) => {
        if (report.type === 'inbound-rtp' && report.kind === 'audio') {
          console.log('Audio Stats:', {
            packetsReceived: report.packetsReceived,
            bytesReceived: report.bytesReceived,
            packetsLost: report.packetsLost,
            jitter: report.jitter,
          });
        }
      });
    });
  }, 2000);
}

function logLastKnownGoodConnection() {
  if (!pc) return;

  const connectionInfo = {
      timestamp: new Date().toISOString(),
      connectionState: pc.connectionState,
      iceConnectionState: pc.iceConnectionState,
      iceGatheringState: pc.iceGatheringState,
      signalingState: pc.signalingState
  };

  console.log('[CLEANUP] Last known connection state:', connectionInfo);

  pc.getStats().then(stats => {
      const lastStats = {
          candidatePairs: [],
          localCandidates: [],
          remoteCandidates: [],
          inboundRTP: []
      };

      stats.forEach(stat => {
          switch (stat.type) {
              case 'candidate-pair':
                  if (stat.state === 'succeeded') {
                      lastStats.candidatePairs.push({
                          state: stat.state,
                          localCandidateId: stat.localCandidateId,
                          remoteCandidateId: stat.remoteCandidateId,
                          bytesReceived: stat.bytesReceived,
                          roundTripTime: stat.currentRoundTripTime
                      });
                  }
                  break;
              case 'local-candidate':
                  lastStats.localCandidates.push({
                      type: stat.candidateType,
                      protocol: stat.protocol,
                      address: stat.address,
                      port: stat.port
                  });
                  break;
              case 'remote-candidate':
                  lastStats.remoteCandidates.push({
                      type: stat.candidateType,
                      protocol: stat.protocol,
                      address: stat.address,
                      port: stat.port
                  });
                  break;
              case 'inbound-rtp':
                  if (stat.kind === 'audio') {
                      lastStats.inboundRTP.push({
                          packetsReceived: stat.packetsReceived,
                          packetsLost: stat.packetsLost,
                          jitter: stat.jitter,
                          bytesReceived: stat.bytesReceived
                      });
                  }
                  break;
          }
      });

      console.log('[CLEANUP] Last known connection stats:', lastStats);
  });
}

document.addEventListener('DOMContentLoaded', function() {
    const synthButton = document.getElementById('synthButton');
    if (synthButton) {
        synthButton.addEventListener('click', handleSynthClick);
    }
    
    // Move your existing event listener inside DOMContentLoaded
    document.body.addEventListener('htmx:configRequest', evt => {
        evt.detail.headers['X-Session-ID'] = sessionID;
    });
});

// register supercollider syntax highlighting
Prism.languages.supercollider = {
    'comment': {
        pattern: /\/\/.*|\/\*[\s\S]*?\*\//,
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
        pattern: /\b(?:SynthDef|Out|Mix|Pan2|SinOsc|LFNoise1|EnvGen|Env|Array|RLPF|BPF|HPF|GVerb|Splay|Limiter|Saw|WhiteNoise|PinkNoise|BrownNoise|DelayN|LocalIn|LocalOut|Dust|Decay|FreqShift|PitchShift|tanh)\b\.?(?=\()/,
        greedy: true
    },
    'number': /\b-?(?:0x[\da-f]+|\d*\.?\d+(?:e[+-]?\d+)?)\b/i,
    'keyword': /\b(?:var|arg|kr|ar|mul|add|range|exprange|fill|new)\b/,
    'operator': /[-+*\/=!<>]=?|[&|^~]|\b(?:and|or|not)\b/,
    'punctuation': /[{}[\];(),.:]/
};

function monitorIceCandidates(pc) {
  let relayFound = false;
  
  pc.addEventListener('icegatheringstatechange', () => {
      if (pc.iceGatheringState === 'complete' && !relayFound) {
          console.error('ICE gathering completed without finding relay candidates');
          pc.getStats().then(stats => {
              const candidates = [];
              stats.forEach(stat => {
                  if (stat.type === 'local-candidate') {
                      candidates.push({
                          type: stat.candidateType,
                          protocol: stat.protocol,
                          address: stat.address
                      });
                  }
              });
              console.log('Final ICE candidates:', candidates);
          });
      }
  });

  return (candidate) => {
      if (candidate.candidate.includes(' relay ')) {
          relayFound = true;
          console.log('Relay candidate found:', candidate.candidate);
      }
  };
}

function monitorTurnConnectivity(pc) {
  pc.getStats().then(stats => {
      stats.forEach(stat => {
          if (stat.type === 'remote-candidate') {
              console.log('TURN Candidate:', {
                  type: stat.candidateType,
                  protocol: stat.protocol,
                  address: stat.address,
                  port: stat.port,
                  turnServer: stat.relayProtocol ? {
                      protocol: stat.relayProtocol,
                      address: stat.relayAddress,
                      port: stat.relayPort
                  } : null
              });
          }
      });
  });
}

function testTurnServer(turnConfig) {
  console.log('[TURN] Testing TURN server connectivity...');
  const pc = new RTCPeerConnection({
    iceServers: [turnConfig],
    iceTransportPolicy: 'relay'
  });
  
  pc.onicecandidate = (e) => {
    if (e.candidate) {
      console.log('[TURN] Candidate gathered:', {
        type: e.candidate.type,
        protocol: e.candidate.protocol,
        address: e.candidate.address,
        port: e.candidate.port,
        raw: e.candidate.candidate
      });
    }
  };

  pc.onicegatheringstatechange = () => {
    console.log('[TURN] ICE gathering state:', pc.iceGatheringState);
  };

  // Create data channel to trigger ICE gathering
  pc.createDataChannel('test');
  pc.createOffer().then(offer => pc.setLocalDescription(offer));
  
  return new Promise((resolve) => {
    setTimeout(() => {
      const stats = {
        gatheredCandidates: [],
        gatheringState: pc.iceGatheringState,
        connectionState: pc.connectionState
      };
      pc.getStats().then(report => {
        report.forEach(stat => {
          if (stat.type === 'local-candidate') {
            stats.gatheredCandidates.push(stat);
          }
        });
        console.log('[TURN] Test results:', stats);
        pc.close();
        resolve(stats);
      });
    }, 5000);
  });
}