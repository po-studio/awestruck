let pc;

document.getElementById('toggleConnection').addEventListener('click', async function () {
  if (!pc || pc.connectionState === 'closed' || pc.connectionState === 'failed') {
    // Start or retry the connection
    this.textContent = 'Connecting...';
    this.disabled = true;
    console.log("Stream starting...");

    pc = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
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

    pc.ontrack = function (event) {
      console.log('Track received:', event.track.kind);
      var container = document.getElementById('container');
      var el = container.querySelector(event.track.kind); // Find an existing element of the same kind (audio or video)

      if (!el) {
        el = document.createElement(event.track.kind);
        el.autoplay = true;
        el.controls = true;
        container.appendChild(el);
        console.log('New audio element added to the document.');
      } else {
        console.log('Updating existing audio element.');
      }

      el.srcObject = event.streams[0]; // Set or update the source object
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

    // Add audio transceiver
    pc.addTransceiver('audio', { 'direction': 'recvonly' });
  } else {
    // Disconnect if connected
    this.textContent = 'Disconnecting...';
    this.disabled = true;
    console.log("Stopping connection...");
    stopSynthesis().then(() => {
      if (pc) {
        document.getElementById('toggleConnection').classList.remove('button-disconnect');
        
        pc.close();
        pc = null; // Reset the peer connection
        
        this.textContent = 'Stream New Synth';
        this.disabled = false;
      }
    });
  }
});

async function sendOffer(sdp) {
  const response = await fetch('/offer', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ sdp: sdp, type: 'offer' })
  });

  if (!response.ok) {
    alert('Failed to send the offer to the server');
    return;
  }

  const resp = await response.json();
  const answer = resp;
  console.log("Received answer:", answer);

  try {
    await pc.setRemoteDescription(new RTCSessionDescription(answer));
    console.log("Remote description set successfully.");
  } catch (err) {
    console.error("Failed to set the remote description:", err);
  }
}

async function stopSynthesis() {
  try {
    const response = await fetch('/stop', { method: 'POST' });
    if (response.ok) {
      console.log("All backend processes have been stopped.");
    } else {
      console.log("Failed to stop backend processes.");
    }
  } catch (error) {
    console.error("Error stopping the backend processes:", error);
  }
}
