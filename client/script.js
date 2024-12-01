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
          pc.getReceivers().forEach(receiver => {
            console.log('Track:', receiver.track.kind, 'State:', receiver.track.readyState);
          });
          
          // Start connection quality monitoring
          setInterval(() => {
            pc.getStats().then(stats => {
              stats.forEach(report => {
                if (report.type === 'candidate-pair' && report.state === 'succeeded') {
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
              });
            });
          }, 1000);
          
          // Update UI
          document.getElementById('toggleConnection').textContent = 'Disconnect';
          document.getElementById('toggleConnection').classList.add('button-disconnect');
          document.getElementById('toggleConnection').disabled = false;
          break;
          
        case 'disconnected':
          console.log('Connection disconnected. Last known state:', states);
          pc.getStats().then(stats => {
            stats.forEach(report => {
              if (report.type === 'candidate-pair' && report.state === 'succeeded') {
                console.log('Last known good connection:', {
                  localCandidate: report.localCandidateId,
                  remoteCandidate: report.remoteCandidateId,
                  lastPacketReceived: report.lastPacketReceivedTimestamp,
                  bytesReceived: report.bytesReceived
                });
              }
            });
          });
          break;
          
        case 'failed':
          console.error('Connection failed:', states);
          document.getElementById('toggleConnection').classList.remove('button-disconnect');
          document.getElementById('toggleConnection').textContent = 'Failed to Connect - Retry?';
          document.getElementById('toggleConnection').disabled = false;
          break;
          
        case 'closed':
          console.log('Connection closed cleanly');
          document.getElementById('toggleConnection').classList.remove('button-disconnect');
          document.getElementById('toggleConnection').textContent = 'Stream New Synth';
          document.getElementById('toggleConnection').disabled = false;
          break;
      }
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
          console.error('Audio error:', e);
          // Try recreating the audio element
          const newAudioElement = document.createElement('audio');
          newAudioElement.srcObject = audioElement.srcObject;
          newAudioElement.autoplay = true;
          newAudioElement.controls = true;
          audioElement.parentNode.replaceChild(newAudioElement, audioElement);
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
        const candidateStr = event.candidate.candidate;
        const parts = candidateStr.split(' ');
        const type = parts[7];
        
        console.log("ICE candidate details:", {
          type,
          protocol: parts[2],
          ip: parts[4], 
          port: parts[5],
          isFiltered: isProduction && type !== 'relay',
          fullCandidate: candidateStr
        });
        
        if (isProduction && type !== 'relay') {
          console.warn('Filtered non-relay candidate');
          return;
        }
        
        sendIceCandidate(event.candidate);
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

    pc.onconnectionstatechange = function() {
      if (pc.connectionState === 'connected') {
        setInterval(() => {
          pc.getStats().then(stats => {
            stats.forEach(report => {
              if (report.type === 'candidate-pair' && report.state === 'succeeded') {
                console.log('Connection Quality:', {
                  currentRoundTripTime: report.currentRoundTripTime,
                  availableOutgoingBitrate: report.availableOutgoingBitrate,
                  bytesReceived: report.bytesReceived
                });
              }
            });
          });
        }, 5000);
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
    const iceServers = pc.getConfiguration().iceServers;
    
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
  const requestBody = {
    candidate: {
      candidate: candidate.candidate,
      sdpMid: candidate.sdpMid,
      sdpMLineIndex: candidate.sdpMLineIndex,
      usernameFragment: candidate.usernameFragment
    }
  };

  const response = await fetch('/ice-candidate', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Session-ID': sessionID
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`Failed to send ICE candidate: ${response.status}`);
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
          "turn:turn.awestruck.io:3478?transport=udp",
          "turn:turn.awestruck.io:3478?transport=tcp",
          "turns:turn.awestruck.io:5349?transport=tcp"
        ],
        username: credentials.username,
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

const isProduction = window.location.hostname !== 'localhost';

async function validateTurnConfig() {
  if (!isProduction) {
    return {
      iceServers: [{
        urls: [
          "stun:localhost:3478",
          "turn:localhost:3478",
          "turns:localhost:5349"
        ],
        username: "awestruck",
        credential: "password"
      }],
      iceTransportPolicy: 'all'
    };
  }
  
  const turnServers = await fetchTurnCredentials();
  return {
    iceServers: turnServers.map(server => ({
      ...server,
      preferUdp: true
    })),
    iceTransportPolicy: 'relay',
    iceCandidatePoolSize: 1
  };
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