const sessionID = getSessionID();

function getSessionID() {
  let sessionID = sessionStorage.getItem('sessionID');
  if (!sessionID) {
    sessionID = 'sid_' + Math.random().toString(36).substr(2, 9);
    sessionStorage.setItem('sessionID', sessionID);
  }
  return sessionID;
}

let pc;
let pendingIceCandidates = [];
let isConnectionEstablished = false;
let audioContext = null;
let audioElement = null;

let qualityMonitorInterval;
let audioStatsInterval;
let audioLevelsInterval;
let trackStatsInterval;

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
    // iceTransportPolicy: 'relay',
    // NB: for local dev, do not use relay
    // Docker networking can interfere with NAT traversal when forcing relay
    // ideally local would mirror deployed environments more closely,
    // but this is a good workaround for now
    iceTransportPolicy: 'all',
    iceCandidatePoolSize: 1,
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
    iceCandidatePoolSize: 1,
  }
};

const isProduction = window.location.hostname !== 'localhost';

document.getElementById('toggleConnection').addEventListener('click', async function() {
  if (!pc || pc.connectionState === 'closed' || pc.connectionState === 'failed') {
    updateToggleButton({ text: 'Connecting...', disabled: true });
    console.log("Stream starting...");
    
    const config = isProduction ? TURN_CONFIG.production : TURN_CONFIG.development;

    console.log("Using config:", config);

    await setupPeerConnection(config);
    // Further connection logic like offer creation/negotiation will happen on negotiationneeded
  } else {
    updateToggleButton({ text: 'Disconnecting...', disabled: true });
    console.log("Stopping connection...");
    try {
      await stopSynthesis();
      handleDisconnect();
    } catch (error) {
      console.error("Failed to stop synthesis:", error);
      updateToggleButton({ text: 'Error - Try Again', disabled: false });
    }
  }
});

function updateToggleButton({ text, disabled = false, disconnectStyle = false }) {
  const toggleBtn = document.getElementById('toggleConnection');
  toggleBtn.textContent = text;
  toggleBtn.disabled = disabled;
  if (disconnectStyle) {
    toggleBtn.classList.add('button-disconnect');
  } else {
    toggleBtn.classList.remove('button-disconnect');
  }
}

function handleDisconnect() {
  const sessionInfo = {
    sessionId: sessionID,
    lastConnectionState: pc ? pc.connectionState : 'none',
    lastIceState: pc ? pc.iceConnectionState : 'none',
    timeStamp: new Date().toISOString()
  };
  
  console.log('Cleaning up session:', sessionInfo);
  
  clearInterval(qualityMonitorInterval);
  clearInterval(audioStatsInterval);
  clearInterval(audioLevelsInterval);
  clearInterval(trackStatsInterval);
  
  if (iceCheckingTimeout) {
    clearTimeout(iceCheckingTimeout);
    iceCheckingTimeout = null;
  }
  
  if (audioElement) {
    audioElement.srcObject = null;
    audioElement.remove();
    audioElement = null;
  }
  
  if (audioContext) {
    audioContext.close().then(() => {
      audioContext = null;
      console.log('Audio context closed successfully');
    }).catch(err => {
      console.error('Error closing audio context:', err);
    });
  }
  
  if (pc) {
    logLastKnownGoodConnection();
    pc.close();
    pc = null;
  }
  
  // Clear the code display with a fade out effect
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
  
  updateToggleButton({ text: 'Generate Synth', disabled: false, disconnectStyle: false });
  isConnectionEstablished = false;
}

async function setupPeerConnection(config) {
  pc = new RTCPeerConnection(config);

  pc.onconnectionstatechange = onConnectionStateChange;
  pc.onicecandidate = onIceCandidate;
  pc.oniceconnectionstatechange = onIceConnectionStateChange;
  pc.onicegatheringstatechange = onIceGatheringStateChange;
  pc.onnegotiationneeded = onNegotiationNeeded;
  pc.ontrack = onTrack;

  pc.addTransceiver('audio', { direction: 'recvonly' });
  return pc;
}

function onConnectionStateChange() {
  if (!pc) {
    console.warn('Connection state change called but pc is null');
    return;
  }

  const states = {
    connectionState: pc.connectionState,
    iceConnectionState: pc.iceConnectionState,
    iceGatheringState: pc.iceGatheringState,
    signalingState: pc.signalingState,
  };
  console.log('Connection state change:', states);

  switch (pc.connectionState) {
    case 'connected':
      console.log('Connection established, checking media tracks...');
      pc.getReceivers().forEach((receiver) => {
        console.log('Track:', receiver.track.kind, 'State:', receiver.track.readyState);
      });

      startConnectionQualityMonitoring();
      startAudioStatsMonitoring();

      updateToggleButton({ text: 'Disconnect', disconnectStyle: true, disabled: false });
      
      // Fetch and display the synth code
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
        .then(code => typeCode(code))
        .catch(error => console.error('Failed to fetch synth code:', error));
      break;

    case 'disconnected':
      console.log('Connection disconnected. Last known state:', states);
      logLastKnownGoodConnection();
      clearCode();
      break;

    case 'failed':
      console.error('Failed:', states);
      updateToggleButton({ text: 'Generate Synth', disabled: false });
      clearCode();
      break;

    case 'closed':
      console.log('Connection closed cleanly');
      updateToggleButton({ text: 'Stream New Synth', disabled: false });
      clearCode();
      break;
  }
}

let iceCheckingTimeout = null;

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

  if (pc.iceConnectionState === 'checking' || pc.iceConnectionState === 'failed') {
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
        console.warn('ICE checking timeout - forcing reconnection');
        handleDisconnect();
        initConnection();
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
    
    if (!isProduction) {
      console.log('Development environment - immediate cleanup on failure');
      handleDisconnect();
      updateToggleButton({ text: 'Connection Failed - Retry?', disabled: false });
      return;
    }
    
    if (pc) {
      pc.restartIce();
      setTimeout(() => {
        if (pc && pc.iceConnectionState === 'failed') {
          handleDisconnect();
          updateToggleButton({ text: 'Connection Failed - Retry?', disabled: false });
        }
      }, 3000);
    }
  }
}

function onIceCandidate(event) {
  if (event.candidate) {
    pendingIceCandidates.push(event.candidate);
    sendIceCandidate(event.candidate);
  }
}

function onIceGatheringStateChange() {
  console.log(`ICE gathering state: ${pc.iceGatheringState}`);
  if (pc.iceGatheringState === 'complete') {
    console.log('Final SDP with ICE candidates:', pc.localDescription.sdp);
  }
}

async function onNegotiationNeeded() {
  if (sessionID) {
    try {
      const response = await fetch('/stop', {
        method: 'POST',
        headers: {
          'X-Session-ID': sessionID
        }
      });
      if (!response.ok) {
        throw new Error('Failed to cleanup previous session');
      }
      await new Promise(resolve => setTimeout(resolve, 2000));
      
      // Create and send offer
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      console.log("Local description set, sending offer to server");
      await sendOffer(pc.localDescription);
    } catch (error) {
      console.error('Negotiation failed:', error);
      handleDisconnect();
      return;
    }
  }
}

function onTrack(event) {
  if (event.track.kind === 'audio') {
    setupAudioTrack(event.track);
  }
}

function setupAudioTrack(track) {
  try {
    console.log('Audio track received:', track);
    console.log('Track Settings:', track.getSettings());
    
    // Create audio context with fallback
    const AudioContext = window.AudioContext || window.webkitAudioContext;
    const audioContext = new AudioContext({
      latencyHint: 'interactive',
      sampleRate: 48000,
    });

    // Log audio context state immediately after creation
    console.log('Audio Context State:', audioContext.state);

    // Create and configure audio element
    const audioElement = document.createElement('audio');
    audioElement.srcObject = new MediaStream([track]);
    audioElement.autoplay = true;
    audioElement.volume = 1.0;

    // Log audio element state
    console.log('Audio Element Ready State:', audioElement.readyState);
    console.log('Track Settings:', track.getSettings());

    // Add event listeners for debugging
    audioElement.onplay = () => console.log('Audio playback started');
    audioElement.onpause = () => console.log('Audio playback paused');
    audioElement.onerror = (e) => console.error('Audio element error:', e);
    audioElement.onwaiting = () => console.log('Audio buffering...');
    audioElement.onstalled = () => console.log('Audio stalled');

    // Create audio graph
    const source = audioContext.createMediaStreamSource(new MediaStream([track]));
    const gainNode = audioContext.createGain();
    const analyser = audioContext.createAnalyser();
    
    source.connect(analyser);
    analyser.connect(gainNode);
    gainNode.connect(audioContext.destination);

    // Start monitoring audio levels
    startAudioLevelMonitoring(analyser, track, audioElement);

  } catch (error) {
    console.error('Error in setupAudioTrack:', error);
    // Log detailed information about the track state
    console.log('Track state at error:', {
      enabled: track.enabled,
      muted: track.muted,
      readyState: track.readyState,
      id: track.id
    });
  }
}

function startAudioLevelMonitoring(analyser, track, audioElement) {
  const dataArray = new Uint8Array(analyser.frequencyBinCount);
  audioLevelsInterval = setInterval(() => {
    try {
      analyser.getByteFrequencyData(dataArray);
      const average = dataArray.reduce((a, b) => a + b) / dataArray.length;
      console.log('Audio RMS level:', average.toFixed(6));
      console.log('Track state:', {
        enabled: track.enabled,
        muted: track.muted,
        readyState: track.readyState
      });
      console.log('Audio element state:', {
        currentTime: audioElement.currentTime,
        paused: audioElement.paused,
        volume: audioElement.volume,
        muted: audioElement.muted
      });
    } catch (error) {
      console.error('Error monitoring audio levels:', error);
      clearInterval(audioLevelsInterval);
    }
  }, 1000);
}

async function sendOffer(offer) {
  try {
    const iceServers = pc.getConfiguration().iceServers;

    console.log('Sending offer with ICE servers:', {
      count: iceServers.length,
      servers: iceServers.map((server) => ({
        urls: server.urls,
        hasCredentials: !!(server.username && server.credential),
      })),
    });

    const browserOffer = {
      sdp: btoa(JSON.stringify(offer)),
      type: 'offer',
      iceServers: iceServers,
    };

    const response = await fetch('/offer', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID,
      },
      body: JSON.stringify(browserOffer),
    });

    if (!response.ok) {
      const errorText = await response.text();
      console.error("Server error response:", {
        status: response.status,
        statusText: response.statusText,
        body: errorText,
      });
      throw new Error(`Server returned ${response.status}: ${errorText}`);
    }

    const rawResponse = await response.text();
    console.log("Raw server response:", rawResponse);

    let answer;
    try {
      answer = JSON.parse(rawResponse);
    } catch (parseError) {
      console.error("Failed to parse server response:", {
        error: parseError,
        rawResponse: rawResponse,
      });
      throw new Error(`Invalid JSON response: ${parseError.message}`);
    }

    console.log("Parsed answer:", {
      type: answer.type,
      sdpLength: answer.sdp?.length,
      sdpPreview: answer.sdp?.substring(0, 100) + '...',
    });

    if (!answer || !answer.sdp || !answer.type) {
      console.error("Invalid answer format:", answer);
      throw new Error("Invalid answer format received from server");
    }

    await pc.setRemoteDescription(new RTCSessionDescription(answer));
    console.log("Remote description set successfully", {
      connectionState: pc.connectionState,
      signalingState: pc.signalingState,
      iceGatheringState: pc.iceGatheringState,
      iceConnectionState: pc.iceConnectionState,
    });

    isConnectionEstablished = true;

    if (pendingIceCandidates.length > 0) {
      console.log(`Processing ${pendingIceCandidates.length} pending ICE candidates`);
      for (const candidate of pendingIceCandidates) {
        await sendIceCandidate(candidate);
      }
      pendingIceCandidates = [];
    }
  } catch (e) {
    console.error("Error in sendOffer:", {
      error: e,
      connectionState: pc?.connectionState,
      signalingState: pc?.signalingState,
      iceGatheringState: pc?.iceGatheringState,
      iceConnectionState: pc?.iceConnectionState,
    });
    throw e;
  }
}

async function sendIceCandidate(candidate) {
  try {
    const requestBody = {
      candidate: {
        candidate: candidate.candidate,
        sdpMid: candidate.sdpMid,
        sdpMLineIndex: candidate.sdpMLineIndex,
        usernameFragment: candidate.usernameFragment,
      },
    };

    const response = await fetch('/ice-candidate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID,
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      console.warn(`ICE candidate send failed with status ${response.status}`, candidate);
      return; // Don't throw, just log and continue
    }
  } catch (error) {
    console.warn('Failed to send ICE candidate:', error);
    // Don't throw, just log the error
  }
}

async function stopSynthesis() {
  try {
    const response = await fetch('/stop', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID
      }
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to stop synthesis: ${response.status} ${errorText}`);
    }

    const result = await response.text();
    console.log("Backend processes stopped:", result);
    isConnectionEstablished = false;
    pendingIceCandidates = [];
  } catch (error) {
    console.error("Error stopping the backend processes:", error);
    throw error;
  }
}

// Monitoring and logging helpers
function startConnectionQualityMonitoring() {
  clearInterval(qualityMonitorInterval);
  qualityMonitorInterval = setInterval(() => {
    if (!pc) {
      clearInterval(qualityMonitorInterval);
      return;
    }
    pc.getStats().then((stats) => {
      stats.forEach((report) => {
        if (report.type === 'candidate-pair' && report.state === 'succeeded') {
          console.log('Connection Quality:', {
            currentRoundTripTime: report.currentRoundTripTime,
            availableOutgoingBitrate: report.availableOutgoingBitrate,
            bytesReceived: report.bytesReceived,
            protocol: report.protocol,
            relayProtocol: report.relayProtocol,
            localCandidateType: report.localCandidateType,
            remoteCandidateType: report.remoteCandidateType,
          });
        }
      });
    });
  }, 1000);
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
  pc.getStats().then((stats) => {
    stats.forEach((report) => {
      if (report.type === 'candidate-pair' && report.state === 'succeeded') {
        console.log('Last known good connection:', {
          localCandidate: report.localCandidateId,
          remoteCandidate: report.remoteCandidateId,
          lastPacketReceived: report.lastPacketReceivedTimestamp,
          bytesReceived: report.bytesReceived,
        });
      }
    });
  });
}

function monitorAudioLevels(analyser, track, audioElement) {
  clearInterval(audioLevelsInterval);
  const dataArray = new Float32Array(analyser.frequencyBinCount);
  
  audioLevelsInterval = setInterval(() => {
    if (!track || track.readyState === 'ended') {
      clearInterval(audioLevelsInterval);
      return;
    }
    
    analyser.getFloatTimeDomainData(dataArray);
    const rms = Math.sqrt(dataArray.reduce((acc, val) => acc + val * val, 0) / dataArray.length);
    console.log('Audio RMS level:', rms.toFixed(6));
    console.log('Track state:', {
      enabled: track.enabled,
      muted: track.muted,
      readyState: track.readyState,
    });
    console.log('Audio element state:', {
      currentTime: audioElement.currentTime,
      paused: audioElement.paused,
      volume: audioElement.volume,
      muted: audioElement.muted,
    });
  }, 1000);
}

function monitorTrackStats(track) {
  clearInterval(trackStatsInterval);
  trackStatsInterval = setInterval(() => {
    if (!pc || !track || track.readyState === 'ended') {
      clearInterval(trackStatsInterval);
      return;
    }
    pc.getStats(track).then((stats) => {
      stats.forEach((report) => {
        if (report.type === 'inbound-rtp' && report.kind === 'audio') {
          console.log('Audio RTP Stats:', {
            packetsReceived: report.packetsReceived,
            packetsLost: report.packetsLost,
            jitter: report.jitter,
            bytesReceived: report.bytesReceived,
            timestamp: report.timestamp,
          });
        }
      });
    });
  }, 1000);
}

// ensures text and scrolling stay perfectly synchronized
// slows down typing speed for better readability
// uses immediate scrolling to prevent lag
async function typeCode(code) {
  const codeDisplay = document.getElementById('codeDisplay');
  codeDisplay.textContent = '';
  codeDisplay.classList.add('visible');
  
  // Only add syntax highlighting class if Prism is available
  if (window.Prism) {
    codeDisplay.classList.add('language-supercollider');
  }
  
  const chunkSize = 20; // Smaller chunks for smoother typing
  for (let i = 0; i < code.length; i += chunkSize) {
    const chunk = code.slice(i, i + chunkSize);
    codeDisplay.textContent += chunk;
    
    // Use immediate scrolling to stay in sync with typing
    codeDisplay.scrollTop = codeDisplay.scrollHeight;
    
    await new Promise(resolve => setTimeout(resolve, 25)); // Slower typing speed
  }
  
  // Apply syntax highlighting after typing is complete if Prism is available
  if (window.Prism && typeof window.Prism.highlightElement === 'function') {
    try {
      Prism.highlightElement(codeDisplay);
    } catch (e) {
      console.warn('Prism syntax highlighting failed:', e);
    }
    codeDisplay.scrollTop = codeDisplay.scrollHeight;
  }
}

function clearCode() {
  const codeDisplay = document.getElementById('codeDisplay');
  codeDisplay.textContent = '';
  codeDisplay.style.opacity = '1'; // Reset opacity
  codeDisplay.style.transition = ''; // Clear any transition
  codeDisplay.classList.remove('visible');
  if (window.Prism) {
    codeDisplay.classList.remove('language-supercollider');
  }
}