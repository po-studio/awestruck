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

    pc = new RTCPeerConnection(
      isProduction 
        ? { iceServers: await fetchTurnCredentials() }
        : TURN_CONFIG.development
    );

    pc.onconnectionstatechange = function (event) {
      console.log(`Connection state change: ${pc.connectionState}`);
      if (pc.connectionState === 'connected') {
        document.getElementById('toggleConnection').textContent = 'Disconnect';
        document.getElementById('toggleConnection').classList.add('button-disconnect');
        document.getElementById('toggleConnection').disabled = false;
      } else if (pc.connectionState === 'failed') {
        document.getElementById('toggleConnection').classList.remove('button-disconnect');
        document.getElementById('toggleConnection').textContent = 'Failed to Connect - Retry?';
        document.getElementById('toggleConnection').disabled = false;
      } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'closed') {
        document.getElementById('toggleConnection').classList.remove('button-disconnect');
        document.getElementById('toggleConnection').textContent = 'Stream Synth';
        document.getElementById('toggleConnection').disabled = false;
      }
    };

    pc.ontrack = function (event) {
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
          sdpMid: event.candidate.sdpMid,
          sdpMLineIndex: event.candidate.sdpMLineIndex,
          usernameFragment: event.candidate.usernameFragment
        });
        sendIceCandidate(event.candidate);
      } else {
        console.log("End of ICE candidates gathering");
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
    });
  }
});

async function sendOffer(offer) {
  if (!pc.connectionState || pc.connectionState === "closed") {
    console.error("Connection not established");
    return;
  }

  try {
    const browserOffer = {
      sdp: offer.sdp,
      type: offer.type,
      iceServers: [] // Server will handle ICE servers
    };

    const response = await fetch("/webrtc", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Session-ID": sessionID
      },
      body: JSON.stringify(browserOffer)
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Server returned ${response.status}: ${errorText}`);
    }

    const answer = await response.json();
    console.log("Received answer:", answer);

    if (!answer || !answer.sdp || !answer.type) {
      throw new Error("Invalid answer format received from server");
    }

    await pc.setRemoteDescription(new RTCSessionDescription(answer));
    isConnectionEstablished = true;

    // Process any pending ICE candidates
    if (pendingIceCandidates.length > 0) {
      console.log(`Processing ${pendingIceCandidates.length} pending ICE candidates`);
      for (const candidate of pendingIceCandidates) {
        await sendIceCandidate(candidate);
      }
      pendingIceCandidates = [];
    }
  } catch (e) {
    console.error("Error in sendOffer:", e);
    throw e; // Let the caller handle the error
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
        'X-Session-ID': sessionID
      }
    });
    if (response.ok) {
      console.log("All backend processes have been stopped.");
    } else {
      console.log("Failed to stop backend processes.");
    }
  } catch (error) {
    console.error("Error stopping the backend processes:", error);
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
      return [{
        urls: credentials.urls || [
          "stun:turn.awestruck.io:3478",
          "turn:turn.awestruck.io:3478",
          "turns:turn.awestruck.io:5349"
        ],
        username: credentials.username,
        credential: credentials.password
      }];
    } catch (error) {
      console.error(`Attempt ${i + 1}/${retries} failed:`, error);
      if (i === retries - 1) throw error;
      await new Promise(resolve => setTimeout(resolve, 1000 * (i + 1)));
    }
  }
}

const TURN_CONFIG = {
  development: {
    iceServers: [{
      urls: [
        "stun:localhost:3478",
        "turn:localhost:3478"
      ],
      username: "test",
      credential: "test123"
    }]
  },
  production: {
    fetchCredentials: true,
    urls: [
      "stun:turn.awestruck.io:3478",
      "turn:turn.awestruck.io:3478",
      "turns:turn.awestruck.io:5349"
    ]
  }
};

const isProduction = window.location.hostname !== 'localhost';