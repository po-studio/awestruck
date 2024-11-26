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

document.getElementById('toggleConnection').addEventListener('click', async function () {
  if (!pc || pc.connectionState === 'closed' || pc.connectionState === 'failed') {

    this.textContent = 'Connecting...';
    this.disabled = true;
    console.log("Stream starting...");

    console.log('Using ICE servers:', isProduction ? 
        await fetchTurnCredentials() : 
        TURN_CONFIG.development
    );

    pc = new RTCPeerConnection(await validateTurnConfig());

    // pc.onconnectionstatechange = function (event) {
    //   console.log(`Connection state change: ${pc.connectionState}`);
    //   if (pc.connectionState === 'connected') {
    //     document.getElementById('toggleConnection').textContent = 'Disconnect';
    //     document.getElementById('toggleConnection').classList.add('button-disconnect');
    //     document.getElementById('toggleConnection').disabled = false;
    //   } else if (pc.connectionState === 'failed') {
    //     document.getElementById('toggleConnection').classList.remove('button-disconnect');
    //     document.getElementById('toggleConnection').textContent = 'Failed to Connect - Retry?';
    //     document.getElementById('toggleConnection').disabled = false;
    //   } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'closed') {
    //     document.getElementById('toggleConnection').classList.remove('button-disconnect');
    //     document.getElementById('toggleConnection').textContent = 'Stream Synth';
    //     document.getElementById('toggleConnection').disabled = false;
    //   }
    // };

    pc.onconnectionstatechange = function() {
      console.log('Connection state change:', {
        connectionState: pc.connectionState,
        iceConnectionState: pc.iceConnectionState,
        iceGatheringState: pc.iceGatheringState,
        signalingState: pc.signalingState
      });
    };

    pc.ontrack = function (event) {
      const audioTrack = event.track;
      
      audioTrack.onunmute = () => {
        console.log('Audio track unmuted!');
      };
      
      audioTrack.onmute = () => {
        console.log('Audio track muted!');
      };
      
      console.log('Track received:', event.track);
      console.log('Track kind:', event.track.kind);
      console.log('Track readyState:', event.track.readyState);
      console.log('Track muted:', event.track.muted);
      console.log('Track enabled:', event.track.enabled);
      
      if (event.track.kind === 'audio') {
        console.log('Audio track received. WebRTC audio connection established.');
        
        var container = document.getElementById('container');
        var audioElement = container.querySelector('audio');
    
        if (!audioElement) {
          audioElement = document.createElement('audio');
          audioElement.autoplay = true;
          audioElement.controls = true;
          audioElement.volume = 0.75;
          audioElement.muted = false;
          container.appendChild(audioElement);
          console.log('New audio element added to the document.');
        } else {
          console.log('Updating existing audio element.');
        }
    
        audioElement.srcObject = event.streams[0];
        
        audioElement.onloadedmetadata = function() {
          console.log('Audio metadata loaded');
        };

        audioElement.onplay = function() {
          console.log('Audio playback started');
        };

        audioElement.onerror = function(e) {
          console.error('Audio playback error:', e);
        };

        // Optional: Add a visual indicator
        var indicator = document.createElement('div');
        indicator.textContent = 'Audio Connected';
        indicator.style.color = 'green';
        container.appendChild(indicator);
      }
    };

    pc.onicecandidate = event => {
      if (event.candidate) {
        console.log("New ICE candidate details:", {
          candidate: event.candidate.candidate,
          type: event.candidate.type,
          candidateType: event.candidate.candidate.split(' ')[7],  // 'typ host/srflx/relay'
          protocol: event.candidate.protocol,
          address: event.candidate.address,
          port: event.candidate.port,
          relatedAddress: event.candidate.relatedAddress,
          relatedPort: event.candidate.relatedPort,
          tcpType: event.candidate.tcpType,
          priority: event.candidate.priority,
          foundation: event.candidate.foundation,
          component: event.candidate.component,
          usernameFragment: event.candidate.usernameFragment,
          raw: event.candidate.candidate
        });
        
        // Only send candidates after connection is established
        if (isConnectionEstablished) {
          sendIceCandidate(event.candidate);
        } else {
          pendingIceCandidates.push(event.candidate);
          console.log(`ICE candidate queued. Total pending: ${pendingIceCandidates.length}`);
        }
      }
    };

    pc.onnegotiationneeded = async () => {
      try {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        console.log("Local description set, sending offer to server");
        await sendOffer(pc.localDescription);
      } catch (err) {
        console.error("Error during negotiation:", err);
        // Reset connection state
        if (pc) {
          pc.close();
          pc = null;
        }
        document.getElementById('toggleConnection').textContent = 'Connection Failed - Retry?';
        document.getElementById('toggleConnection').disabled = false;
        document.getElementById('toggleConnection').classList.remove('button-disconnect');
      }
    };

    pc.addTransceiver('audio', { 'direction': 'recvonly' });

    pc.addEventListener('icegatheringstatechange', () => {
      console.log('ICE gathering state:', pc.iceGatheringState);
    });

    pc.addEventListener('iceconnectionstatechange', () => {
      console.log('ICE connection state:', pc.iceConnectionState);
      console.log('Current ICE candidates:', pc.localDescription.sdp);
    });

    pc.onicegatheringstatechange = () => {
      console.log(`ICE gathering state: ${pc.iceGatheringState}`);
      if (pc.iceGatheringState === 'complete') {
        console.log('Final SDP with ICE candidates:', pc.localDescription.sdp);
      }
    };

    pc.oniceconnectionstatechange = function() {
      console.log('ICE Connection state:', pc.iceConnectionState);
      
      if (pc.iceConnectionState === 'connected') {
        pc.getStats().then(stats => {
          stats.forEach(report => {
            if (report.type === 'candidate-pair' && report.state === 'succeeded') {
              console.log('Active ICE candidate pair:', report);
            }
          });
        });
      }
    };
  } else {

    this.textContent = 'Disconnecting...';
    this.disabled = true;
    console.log("Stopping connection...");
    stopSynthesis().then(() => {
      if (pc) {
        document.getElementById('toggleConnection').classList.remove('button-disconnect');

        pc.close();
        pc = null;

        this.textContent = 'Stream New Synth';
        this.disabled = false;
      }
    }).catch(error => {
      console.error("Failed to stop synthesis:", error);
      this.textContent = 'Error - Try Again';
      this.disabled = false;
    });
  }
});

async function sendOffer(offer) {
  try {
    const iceServers = isProduction ? 
      await fetchTurnCredentials() : 
      TURN_CONFIG.development.iceServers;
    
    console.log('Sending offer with ICE servers:', {
      count: iceServers.length,
      servers: iceServers.map(server => ({
        urls: server.urls,
        hasCredentials: !!(server.username && server.credential)
      }))
    });

    const browserOffer = {
      sdp: btoa(JSON.stringify(offer)),
      type: 'offer',
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
      const errorText = await response.text();
      console.error("Server error response:", {
        status: response.status,
        statusText: response.statusText,
        body: errorText
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
        rawResponse: rawResponse
      });
      throw new Error(`Invalid JSON response: ${parseError.message}`);
    }

    console.log("Parsed answer:", {
      type: answer.type,
      sdpLength: answer.sdp?.length,
      sdpPreview: answer.sdp?.substring(0, 100) + '...'
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
      iceConnectionState: pc.iceConnectionState
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
      iceConnectionState: pc?.iceConnectionState
    });
    throw e;
  }
}

async function sendIceCandidate(candidate) {
  if (!isConnectionEstablished) {
    console.log("Connection not established yet, queuing ICE candidate");
    pendingIceCandidates.push(candidate);
    return;
  }

  const requestBody = {
    candidate: {
      candidate: candidate.candidate,
      sdpMid: candidate.sdpMid,
      sdpMLineIndex: candidate.sdpMLineIndex,
      usernameFragment: candidate.usernameFragment
    }
  };

  console.log("Sending ICE candidate to server:", requestBody);

  try {
    const response = await fetch('/ice-candidate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID
      },
      body: JSON.stringify(requestBody)
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(`Failed to send ICE candidate: ${response.status} ${text}`);
    }
    console.log("Successfully sent ICE candidate to server");
  } catch (err) {
    console.error("Error sending ICE candidate:", {
      error: err.message,
      sessionID: sessionID,
      candidate: candidate
    });
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
    throw error; // Re-throw to be handled by caller
  }
}

async function fetchTurnCredentials(retries = 3) {
  for (let i = 0; i < retries; i++) {
    try {
      const response = await fetch('/turn-credentials', {
        headers: {
          'Content-Type': 'application/json',
          'X-Session-ID': sessionID
        }
      });
      
      if (!response.ok) {
        throw new Error(`Failed to fetch TURN credentials: ${response.statusText}`);
      }
      
      const credentials = await response.json();
      console.log('TURN credentials received:', credentials);
      
      // Use the server-provided credentials
      return [{
        urls: [
          "turn:turn.awestruck.io:3478",
          "turns:turn.awestruck.io:5349"
        ],
        username: credentials.username,  // Use server-provided username
        credential: credentials.password,
        credentialType: 'password'
      }];
    } catch (error) {
      console.error(`TURN credential fetch attempt ${i + 1}/${retries} failed:`, error);
      if (i === retries - 1) throw error;
      await new Promise(resolve => setTimeout(resolve, 1000 * (i + 1)));
    }
  }
}

// const TURN_CONFIG = {
//   development: {
//     iceServers: [{
//       urls: [
//         "stun:localhost:3478",
//         "turn:localhost:3478",
//         "turns:localhost:5349"
//       ],
//       username: "test",
//       credential: "test123"
//     }],
//     iceTransportPolicy: 'relay'
//   },
//   production: {
//     fetchCredentials: true,
//     urls: [
//       "stun:turn.awestruck.io:3478",
//       "turn:turn.awestruck.io:3478",
//       "turns:turn.awestruck.io:5349"
//     ],
//     iceTransportPolicy: 'relay'
//   }
// };

const TURN_CONFIG = {
  development: {
    iceServers: [{
      urls: [
        "stun:localhost:3478",
        "turn:localhost:3478",
        "turns:localhost:5349"
      ],
      username: "test",
      credential: "test123"
    }],
    iceTransportPolicy: 'relay'
  },
  production: {
    iceServers: [{
      urls: [
        "turn:turn.awestruck.io:3478?transport=udp",
        "turns:turn.awestruck.io:5349?transport=tcp"
      ],
      username: "test",
      credential: "password",
      credentialType: 'password'
    }],
    iceTransportPolicy: 'relay'
  }
};

const isProduction = window.location.hostname !== 'localhost';

// async function validateTurnConfig() {
//   const config = isProduction ? 
//     { 
//       iceServers: await fetchTurnCredentials(),
//       iceTransportPolicy: 'relay',
//       iceCandidatePoolSize: 0,
//       bundlePolicy: 'balanced',
//       rtcpMuxPolicy: 'require'
//     } : 
//     TURN_CONFIG.development;
    
//   console.log('TURN Configuration:', {
//     iceServers: config.iceServers.map(server => ({
//       urls: server.urls,
//       hasCredentials: !!(server.username && server.credential)
//     })),
//     iceTransportPolicy: config.iceTransportPolicy
//   });
  
//   return config;
// }

async function validateTurnConfig() {
  const config = isProduction ? 
    TURN_CONFIG.production : 
    TURN_CONFIG.development;
    
  console.log('TURN Configuration:', {
    iceServers: config.iceServers.map(server => ({
      urls: server.urls,
      hasCredentials: !!(server.username && server.credential)
    })),
    iceTransportPolicy: config.iceTransportPolicy
  });
  
  return config;
}

async function testTurnServer() {
  const config = await validateTurnConfig();
  const pc = new RTCPeerConnection(config);
  
  pc.onicecandidate = e => {
    if (e.candidate) {
      const candidateType = e.candidate.candidate.split(' ')[7];
      console.log(`Test ICE Candidate: ${candidateType}`, {
        protocol: e.candidate.protocol,
        address: e.candidate.address,
        raw: e.candidate.candidate
      });
    }
  };
  
  // Create a data channel to trigger ICE gathering
  pc.createDataChannel("test");
  
  try {
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    
    // Wait for ICE gathering to complete
    await new Promise((resolve) => {
      if (pc.iceGatheringState === 'complete') resolve();
      else pc.onicegatheringstatechange = () => {
        if (pc.iceGatheringState === 'complete') resolve();
      };
    });
    
  } finally {
    pc.close();
  }
}