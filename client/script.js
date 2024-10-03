let pc;

document.getElementById('toggleConnection').addEventListener('click', async function () {
  if (!pc || pc.connectionState === 'closed' || pc.connectionState === 'failed') {

    this.textContent = 'Connecting...';
    this.disabled = true;
    console.log("Stream starting...");

    // pc = new RTCPeerConnection({
    //   iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    // });

    pc = new RTCPeerConnection({
      iceServers: [
          {
            urls: "stun:stun.relay.metered.ca:80",
          },
          {
            urls: "turn:global.relay.metered.ca:80",
            username: "b6be1a94a4dbaa7c04a65bc9",
            credential: "FLXvDM76W65uQiLc",
          },
          {
            urls: "turn:global.relay.metered.ca:80?transport=tcp",
            username: "b6be1a94a4dbaa7c04a65bc9",
            credential: "FLXvDM76W65uQiLc",
          },
          {
            urls: "turn:global.relay.metered.ca:443",
            username: "b6be1a94a4dbaa7c04a65bc9",
            credential: "FLXvDM76W65uQiLc",
          },
          {
            urls: "turns:global.relay.metered.ca:443?transport=tcp",
            username: "b6be1a94a4dbaa7c04a65bc9",
            credential: "FLXvDM76W65uQiLc",
          },
      ],
    });

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

    let isNegotiationNeeded = false;

    // pc.ontrack = function (event) {
    //   console.log('Track received:', event.track.kind);
    //   var container = document.getElementById('container');
    //   var el = container.querySelector(event.track.kind);

    //   if (!el) {
    //     el = document.createElement(event.track.kind);
    //     el.autoplay = true;
    //     el.controls = true;
    //     container.appendChild(el);
    //     console.log('New audio element added to the document.');
    //   } else {
    //     console.log('Updating existing audio element.');
    //   }

    //   el.srcObject = event.streams[0];
    // };
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
      if (event.candidate === null && isNegotiationNeeded) {
        isNegotiationNeeded = false;
        let sdp = btoa(JSON.stringify(pc.localDescription));
        console.log("Local Session Description:", sdp);
        sendOffer(sdp);
      } else if (event.candidate) {
        console.log("Sending ICE candidate:", event.candidate);
      } else {
        console.log("End of candidates.");
      }
    };

    pc.onnegotiationneeded = async () => {
      try {
        isNegotiationNeeded = true;
        await pc.setLocalDescription(await pc.createOffer());
      } catch (err) {
        console.error(err);
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

async function sendOffer(sdp) {
  try {
    const response = await fetch('/offer', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionID
      },
      body: JSON.stringify({ sdp: sdp, type: 'offer' })
    });

    if (!response.ok) {
      const errorText = await response.text();
      console.error(`Server responded with status ${response.status}: ${errorText}`);
      alert(`Failed to send the offer to the server. Status: ${response.status}, Error: ${errorText}`);
      return;
    }

    const resp = await response.json();
    const answer = resp;
    console.log("Received answer:", answer);

    await pc.setRemoteDescription(new RTCSessionDescription(answer));
    console.log("Remote description set successfully.");
  } catch (err) {
    console.error("Error in sendOffer:", err);
    alert(`Failed to send the offer to the server: ${err.message}`);
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


function getSessionID() {
  let sessionID = sessionStorage.getItem('sessionID');
  if (!sessionID) {
    sessionID = 'sid_' + Math.random().toString(36).substr(2, 9);
    sessionStorage.setItem('sessionID', sessionID);
  }
  return sessionID;
}


const sessionID = getSessionID();
