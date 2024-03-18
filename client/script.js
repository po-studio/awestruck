let pc;

document.getElementById('startButton').addEventListener('click', async () => {
  console.log("Stream starting...");

  pc = new RTCPeerConnection({
    iceServers: [
      {
        urls: 'stun:stun.l.google.com:19302'
      }
    ]
  });

  pc.onconnectionstatechange = function (event) {
    console.log(`Connection state change: ${pc.connectionState}`);
  };

  let isNegotiationNeeded = false;

  pc.ontrack = function (event) {
    console.log('Track received:', event.track.kind);
    var el = document.createElement(event.track.kind);
    el.srcObject = event.streams[0];
    el.autoplay = true;
    el.controls = true;
    document.body.appendChild(el); // Make sure this is happening
    console.log('Audio element added to the document.');
  };

  pc.onicecandidate = event => {
    if (event.candidate === null && isNegotiationNeeded) {
      isNegotiationNeeded = false; // Reset the flag
      let sdp = btoa(JSON.stringify(pc.localDescription));
      console.log("Local Session Description:", sdp);

      // Here we send the local session description to the server
      sendOffer(sdp);
    } else if (event.candidate) {
      console.log("Sending ICE candidate:", event.candidate);
    } else {
      console.log("End of candidates.");
    }
  };

  pc.onnegotiationneeded = async () => {
    try {
      isNegotiationNeeded = true; // Set the flag to true
      await pc.setLocalDescription(await pc.createOffer());
      // NOTE: The ICE gathering will trigger the onicecandidate event
    } catch (err) {
      console.error(err);
    }
  };

  // Add audio transceiver
  pc.addTransceiver('audio', { 'direction': 'recvonly' });
});

// Function to send the offer to the server
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

  const resp = await response.json(); // This should correctly parse the JSON response
  const answer = resp;
  console.log("Received answer:", answer);

  try {
    await pc.setRemoteDescription(new RTCSessionDescription(answer));
    console.log("Remote description set successfully.");
  } catch (err) {
    console.error("Failed to set the remote description:", err);
  }
}