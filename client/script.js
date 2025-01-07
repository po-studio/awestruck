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
// We removed unused global intervals and the unused global turnConfig variable

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
      return;
  }

  button.textContent = 'Connecting...';
  button.disabled = true;

  try {
      // Dynamically fetch TURN credentials from the server
      const turnResponse = await fetch('/turn-credentials', {
          headers: { 'X-Session-ID': sessionID }
      });
      const fetchedTurnConfig = await turnResponse.json();

      // Initialize WebRTC
      await setupWebRTC(fetchedTurnConfig);

      button.textContent = 'Stop Synth';
      button.classList.add('button-disconnect');
      button.disabled = false;

      // Start polling for code updates
      startCodeMonitoring();
  } catch (error) {
      console.error('Connection failed:', error);
      button.textContent = 'Error - Try Again';
      button.disabled = false;
  }
}

async function setupWebRTC(config) {
  pc = new RTCPeerConnection(config);

  pc.onconnectionstatechange = handleConnectionStateChange;
  pc.onicecandidate = handleICECandidate;
  pc.oniceconnectionstatechange = onIceConnectionStateChange;
  pc.onicegatheringstatechange = onIceGatheringStateChange;
  pc.ontrack = handleTrack;

  pc.addTransceiver('audio', { direction: 'recvonly' });

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

function handleConnectionStateChange() {
  if (!pc) {
    console.warn('Connection state change called but pc is null');
    return;
  }

  const states = {
    connectionState: pc.connectionState,
    iceConnectionState: pc.iceConnectionState,
    iceGatheringState: pc.iceGatheringState,
    signalingState: pc.signalingState
  };
  console.log('Connection state change:', states);

  switch (pc.connectionState) {
    case 'connected':
      console.log('Connection established, checking media tracks...');
      pc.getReceivers().forEach((receiver) => {
        console.log('Track:', receiver.track.kind, 'State:', receiver.track.readyState);
      });
      startConnectionMonitoring();
      break;

    case 'disconnected':
      console.log('Connection disconnected. Last known state:', states);
      logLastKnownGoodConnection();
      break;

    case 'failed':
      console.error('Connection failed:', states);
      cleanupConnection();
      break;

    case 'closed':
      console.log('Connection closed cleanly');
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

function startCodeMonitoring() {
  const codeDisplay = document.getElementById('codeDisplay');

  const pollCode = async () => {
      if (!pc || pc.connectionState === 'closed') return;
      try {
          const response = await fetch('/synth-code', {
              headers: { 'X-Session-ID': sessionID }
          });
          const code = await response.text();
          await typeCode(code);
      } catch (error) {
          console.error('Failed to fetch code:', error);
      }
  };

  // Kick it off once, then poll every 5s
  pollCode();
  codePollingInterval = setInterval(pollCode, 5000);
}

async function typeCode(code) {
  const codeDisplay = document.getElementById('codeDisplay');
  if (!codeDisplay) return;

  clearCode();
  codeDisplay.classList.add('visible', 'language-supercollider');

  const chars = code.split('');
  let currentText = '';

  for (const char of chars) {
      currentText += char;
      codeDisplay.textContent = currentText;
      Prism.highlightElement(codeDisplay);
      // Random typing speed 1-5ms
      await new Promise(resolve => setTimeout(resolve, Math.random() * 4 + 1));
  }
}

function clearCode() {
  const codeDisplay = document.getElementById('codeDisplay');
  if (codeDisplay) {
      codeDisplay.textContent = '';
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

function onIceConnectionStateChange() {
  console.log('ICE Connection State:', pc.iceConnectionState);

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
