let pc = new RTCPeerConnection({
  iceServers: [
    {
      urls: 'stun:stun.l.google.com:19302'
    }
  ]
})
let log = msg => {
  document.getElementById('div').innerHTML += msg + '<br>'
}

pc.ontrack = function (event) {
  var el = document.createElement(event.track.kind)
  el.srcObject = event.streams[0]
  el.autoplay = true
  el.controls = true
}

pc.oniceconnectionstatechange = e => log(pc.iceConnectionState)
pc.onicecandidate = event => {
  if (event.candidate === null) {
    token = btoa(JSON.stringify(pc.localDescription));
    var proto = window.location.protocol;
    var host = window.location.host;

    curl = "curl -X POST " + proto + "//" + host + "/token -d '{\"token\":\"" + token + "\"}'"
    
    document.getElementById('localSessionDescription').value = curl;
  }
}

pc.addTransceiver('audio', {'direction': 'recvonly'})
pc.createOffer().then(d => pc.setLocalDescription(d)).catch(log)

window.startSession = () => {
  let sd = document.getElementById('remoteSessionDescription').value
  if (sd === '') {
   return alert('Session Description must not be empty')
  }

  try {
    pc.setRemoteDescription(new RTCSessionDescription(JSON.parse(atob(sd))))
  } catch (e) {
    alert(e)
  }
}

function copyText() {
  /* Get the text field */
  var copyText = document.getElementById("localSessionDescription");

  /* Select the text field */
  copyText.select();
  copyText.setSelectionRange(0, 99999); /*For mobile devices*/

  /* Copy the text inside the text field */
  document.execCommand("copy");

  /* Alert the copied text */
  alert("Copied the text: " + copyText.value);
}