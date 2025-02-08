<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Awestruck</title>
  <!-- Bungee Shade for Logo -->
  <link
    href="https://fonts.googleapis.com/css2?family=Bungee+Shade&display=swap"
    rel="stylesheet"
  >
  <!-- Space Grotesk for Everything Else -->
  <link
    href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@300;500;700&display=swap"
    rel="stylesheet"
  >
  <style>
    /* Reset */
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    html, body {
      height: 100%;
      width: 100%;
      display: flex;
      flex-direction: row;
      font-family: 'Space Grotesk', sans-serif;
      overflow: hidden; /* remove if you need scrolling */
    }

    /* === LEFT PANEL === */
    .left-panel {
      flex: 1;
      background: #FF0080; /* Neon pink */
      color: #000;
      padding: 30px 20px;
      display: flex;
      flex-direction: column;
      gap: 30px;
      overflow-y: auto;
    }

    .brandmark {
      font-family: 'Bungee Shade', cursive;
      font-size: 5rem;
      letter-spacing: 2px;
      text-transform: uppercase;
    }

    .tagline {
      font-size: 1.6rem;
      font-weight: 700;
      line-height: 1.3;
    }

    .description {
      font-size: 1rem;
      font-weight: 400;
      max-width: 400px;
      line-height: 1.4;
      margin-top: -10px;
    }

    .cta-btn {
      display: inline-block;
      padding: 12px 24px;
      border: none;
      border-radius: 8px;
      background: #000;
      color: #FF0080;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      cursor: pointer;
      transition: transform 0.2s ease-in-out, box-shadow 0.2s ease-in-out;
    }
    .cta-btn:hover {
      transform: skew(-15deg);
      box-shadow: 4px 6px 10px rgba(0,0,0,0.3);
    }

    .features {
      display: flex;
      flex-direction: column;
      gap: 15px;
      max-width: 500px;
    }
    .feature {
      padding-left: 10px;
      border-left: 4px solid #000;
    }
    .feature h2 {
      font-size: 1.1rem;
      font-weight: 700;
      margin-bottom: 5px;
    }
    .feature p {
      font-size: 0.95rem;
      line-height: 1.4;
      color: #111;
    }

    .footer {
      margin-top: auto;
      font-size: 0.8rem;
    }

    /* === RIGHT PANEL === */
    .right-panel {
      flex: 1;
      background: #111; /* Dark background */
      color: #eee;
      padding: 20px;
      overflow-y: auto;
      display: flex;
      flex-direction: column;
      gap: 20px;
    }

    /* A small status bar up top */
    #status {
      display: inline-block;
      padding: 5px 10px;
      background: #333;
      border-radius: 4px;
    }

    /* Logs container with subtle styling */
    .logs-container {
      background-color: #222;
      border-radius: 5px;
      padding: 10px;
      font-size: 0.85rem;
      line-height: 1.3;
      height: 150px;
      overflow-y: auto;
    }
    .log-line {
      margin-bottom: 5px;
      animation: fadeIn 0.3s ease-out;
    }
    @keyframes fadeIn {
      from { opacity: 0; transform: translateX(-10px); }
      to { opacity: 1; transform: translateX(0); }
    }

    /* Circular Play/Stop button */
    .playstop-button {
      width: 60px;
      height: 60px;
      border-radius: 50%;
      border: none;
      font-size: 2rem;  /* large symbol */
      font-weight: 700;
      cursor: pointer;
      transition: background 0.2s, transform 0.2s;
      background: #0f0; /* default for 'stopped' */
      color: #111;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    /* "Stopped" state => green button with '>' icon */
    .playstop-button[data-state="stopped"] {
      background: #0f0;
      color: #111;
    }
    /* "Playing" state => red button with '■' icon */
    .playstop-button[data-state="playing"] {
      background: #f33;
      color: #fff;
    }
    .playstop-button:hover {
      transform: scale(1.05);
    }
  </style>
</head>
<body>

  <!-- LEFT PANEL -->
  <div class="left-panel">
    <div class="brandmark">AWESTRUCK</div>
    <div class="tagline">Design Sound, in the Moment.</div>
    <div class="description">
      Awestruck is a real-time audio engine that lets you shape and stream 
      custom synths on the fly, powered by AI and built for collaborative creativity.
    </div>
    <button class="cta-btn">Get Early Access</button>

    <div class="features">
      <div class="feature">
        <h2>Live Synthesis</h2>
        <p>Generate and stream new sounds instantly using our server-side audio engine.</p>
      </div>
      <div class="feature">
        <h2>AI-Compiled Programs</h2>
        <p>Use text prompts to produce original SuperCollider synths with large language models.</p>
      </div>
      <div class="feature">
        <h2>Shared Collaboration</h2>
        <p>Co-create in real time, tweak parameters with others, and track your unique sounds on-chain if desired.</p>
      </div>
    </div>

    <div class="footer">
      &copy; 2025 Aurafex Technologies
    </div>
  </div>

  <!-- RIGHT PANEL -->
  <div class="right-panel">
    <!-- Status bar -->
    <div id="status">Disconnected</div>

    <!-- Logs container -->
    <div class="logs-container" id="logs">
      <!-- Minimal logs appended here by script -->
    </div>

    <!-- One toggle play/stop button -->
    <button 
      class="playstop-button" 
      id="playstop-button"
      data-state="stopped"
      onclick="toggleSynth(this)"
    >
      &gt;
    </button>
  </div>

  <!-- Inline Script -->
  <script>
  // ==========================================================
  //  Below is the updated script logic
  // ==========================================================

  // Simple session management
  class SessionManager {
      static getSessionId() {
          let sessionId = localStorage.getItem('sessionId');
          if (!sessionId) {
              sessionId = Math.random().toString(36).substring(2, 15);
              localStorage.setItem('sessionId', sessionId);
          }
          return sessionId;
      }
  }

  // Minimal logger with shorter lines in the UI
  class Logger {
      static log(type, message, data) {
          // Create a short message ignoring giant data or truncating it
          const shortMsg = `[${type}] ${message}`;
          console.log(shortMsg, data || '');

          const logsElement = document.getElementById('logs');
          if (logsElement) {
              const line = document.createElement('div');
              line.className = 'log-line';
              // Just show type/message
              line.textContent = shortMsg;
              logsElement.appendChild(line);
              logsElement.scrollTop = logsElement.scrollHeight;
          }
      }

      static webrtc(msg, data) { this.log('WebRTC', msg, data); }
      static ice(msg, data) { this.log('ICE', msg, data); }
      static turn(msg, data) { this.log('TURN', msg, data); }
      static error(msg, data) { this.log('ERROR', msg, data); }
  }

  // Monitor connection states
  class ConnectionMonitor {
      constructor(pc) {
          this.pc = pc;
          this.startTime = Date.now();
          this.intervals = [];
          this.setupListeners();
      }

      setupListeners() {
          this.pc.onconnectionstatechange = () => {
              Logger.webrtc(`Connection state changed`, {
                  state: this.pc.connectionState,
                  elapsed: (Date.now() - this.startTime) / 1000
              });
              this.updateStatus();
          };

          this.pc.oniceconnectionstatechange = () => {
              Logger.ice(`ICE state changed`, {
                  state: this.pc.iceConnectionState,
                  elapsed: (Date.now() - this.startTime) / 1000
              });
          };

          this.pc.onicegatheringstatechange = () => {
              Logger.ice(`Gathering state changed`, {
                  state: this.pc.iceGatheringState,
                  elapsed: (Date.now() - this.startTime) / 1000
              });
          };

          this.pc.onicecandidate = (event) => {
              if (event.candidate) {
                  Logger.ice(`New candidate`, {
                      candidate: event.candidate.candidate,
                      sdpMid: event.candidate.sdpMid,
                      sdpMLineIndex: event.candidate.sdpMLineIndex
                  });
              }
          };
      }

      updateStatus() {
          const status = document.getElementById('status');
          if (!status) return;

          const colors = {
              'new': '#eee',
              'connecting': '#ff9',
              'connected': '#9f9',
              'disconnected': '#f99',
              'failed': '#f66',
              'closed': '#999'
          };

          status.textContent = this.pc.connectionState;
          status.style.backgroundColor = colors[this.pc.connectionState] || '#eee';
      }

      stop() {
          this.intervals.forEach(interval => {
              if (interval) {
                  clearInterval(interval);
              }
          });
          this.intervals = [];
          Logger.webrtc('Stopped connection monitoring');
      }

      async getStats() {
          const stats = await this.pc.getStats();
          const result = {};
          
          stats.forEach(stat => {
              if (['transport', 'candidate-pair', 'local-candidate', 'remote-candidate'].includes(stat.type)) {
                  result[stat.type] = stat;
              }
          });
          return result;
      }
  }

  // Handles audio playback & monitors levels
  class AudioHandler {
      constructor(track) {
          this.track = track;
          this.setupAudioElement();
          this.setupAudioContext();
      }

      setupAudioElement() {
          const audio = new Audio();
          audio.srcObject = new MediaStream([this.track]);
          audio.volume = 1.0;
          audio.autoplay = true;
          
          audio.onerror = (err) => {
              Logger.error('Audio playback error', err);
          };
          audio.onplay = () => {
              Logger.log('AUDIO', 'Playback started');
          };
          
          this.audioElement = audio;
          
          // Attempt to play
          audio.play().catch(err => {
              Logger.error('Audio playback failed', err);
              document.body.addEventListener('click', () => {
                  audio.play().catch(e => Logger.error('Retry playback failed', e));
              }, { once: true });
          });
      }

      setupAudioContext() {
          try {
              const ctx = new (window.AudioContext || window.webkitAudioContext)();
              const source = ctx.createMediaStreamSource(new MediaStream([this.track]));
              const gainNode = ctx.createGain();
              const analyser = ctx.createAnalyser();
              
              gainNode.gain.value = 1.0;
              
              source.connect(gainNode);
              gainNode.connect(analyser);
              gainNode.connect(ctx.destination);
              
              this.audioContext = ctx;
              this.gainNode = gainNode;
              
              this.monitorLevels(analyser);
          } catch (err) {
              Logger.error('Audio context setup failed', err);
          }
      }

      monitorLevels(analyser) {
          const dataArray = new Float32Array(analyser.frequencyBinCount);
          
          const check = () => {
              analyser.getFloatTimeDomainData(dataArray);
              const level = Math.sqrt(dataArray.reduce((acc, val) => acc + val * val, 0) / dataArray.length);

              Logger.log('AUDIO', 'Level', {
                  rms: level.toFixed(4),
                  peak: Math.max(...dataArray.map(Math.abs)).toFixed(4)
              });

              if (level === 0 && this.audioContext?.state === 'suspended') {
                  this.audioContext.resume();
              }
          };

          setInterval(check, 2000);
      }

      setVolume(value) {
          if (this.audioElement) {
              this.audioElement.volume = value;
          }
          if (this.gainNode) {
              this.gainNode.gain.value = value;
          }
      }
  }

  // ICE candidate handling
  class ICEHandler {
      constructor(pc, sessionId) {
          this.pc = pc;
          this.sessionId = sessionId;
          this.pendingCandidates = [];
          this.hasRemoteDescription = false;
          this.setupICEHandling();
      }

      setupICEHandling() {
          this.pc.onicegatheringstatechange = () => {
              Logger.ice('Gathering state changed', {
                  state: this.pc.iceGatheringState
              });
              
              if (this.pc.iceGatheringState === 'complete') {
                  Logger.ice('Gathering completed');
              }
          };

          this.pc.onicecandidate = async (event) => {
              if (!event.candidate) {
                  Logger.ice('Finished gathering candidates');
                  return;
              }

              const candidate = event.candidate;
              Logger.ice('New candidate', {
                  type: this.extractCandidateType(candidate.candidate),
                  protocol: candidate.protocol,
                  address: candidate.address,
                  port: candidate.port
              });

              if (!this.hasRemoteDescription) {
                  Logger.ice('Queueing candidate until remote description is set');
                  this.pendingCandidates.push(candidate);
                  return;
              }

              await this.sendCandidate(candidate);
          };
      }

      extractCandidateType(candidateStr) {
          const match = candidateStr.match(/typ ([a-z]+)/);
          return match ? match[1] : 'unknown';
      }

      async sendCandidate(candidate) {
          try {
              const payload = {
                  candidate: btoa(JSON.stringify({
                      candidate: candidate.candidate,
                      sdpMid: candidate.sdpMid,
                      sdpMLineIndex: candidate.sdpMLineIndex,
                      usernameFragment: candidate.usernameFragment
                  }))
              };

              const response = await fetch('/ice-candidate', {
                  method: 'POST',
                  headers: {
                      'Content-Type': 'application/json',
                      'X-Session-ID': this.sessionId
                  },
                  body: JSON.stringify(payload)
              });

              if (!response.ok) {
                  throw new Error(`Failed to send candidate: ${response.status}`);
              }

              Logger.ice('Successfully sent candidate');
          } catch (error) {
              Logger.error('Failed to send ICE candidate', error);
              this.pendingCandidates.push(candidate);
          }
      }

      setRemoteDescription(hasRemote) {
          this.hasRemoteDescription = hasRemote;
          if (hasRemote && this.pendingCandidates.length > 0) {
              Logger.ice(`Sending ${this.pendingCandidates.length} pending candidates`);
              this.pendingCandidates.forEach(candidate => this.sendCandidate(candidate));
              this.pendingCandidates = [];
          }
      }
  }

  // Helper to set status text
  function setStatusMessage(msg, color = '#eee') {
      const status = document.getElementById('status');
      if (status) {
          status.textContent = msg;
          status.style.backgroundColor = color;
      }
  }

  // Cleanup logic
  async function stopSynth() {
      Logger.webrtc('Stopping synth');
      setStatusMessage('Stopping...', '#f99');
      
      try {
          await cleanupConnection();
          Logger.webrtc('Successfully stopped synth');
          setStatusMessage('Disconnected', '#eee');
      } catch (error) {
          Logger.error('Error stopping synth', error);
          setStatusMessage('Error stopping', '#f66');
      }
  }

  async function cleanupConnection() {
      Logger.webrtc('Starting cleanup');

      try {
          await fetch('/stop', {
              method: 'POST',
              headers: {
                  'Content-Type': 'application/json',
                  'X-Session-ID': SessionManager.getSessionId()
              }
          });
          Logger.webrtc('Sent stop signal to server');
      } catch (error) {
          Logger.error('Failed to send stop signal', error);
      }

      if (window.audioHandler) {
          if (window.audioHandler.audioContext) {
              await window.audioHandler.audioContext.close();
          }
          if (window.audioHandler.audioElement) {
              window.audioHandler.audioElement.pause();
              window.audioHandler.audioElement.srcObject = null;
              window.audioHandler.audioElement.remove();
          }
          window.audioHandler = null;
          Logger.webrtc('Cleaned up audio resources');
      }

      if (window.pc) {
          window.pc.getReceivers().forEach(receiver => {
              if (receiver.track) {
                  receiver.track.stop();
              }
          });
          window.pc.close();
          window.pc = null;
          Logger.webrtc('Closed peer connection');
      }

      if (window.monitor) {
          window.monitor.stop();
          window.monitor = null;
          Logger.webrtc('Stopped monitoring');
      }

      Logger.webrtc('Cleanup complete');
  }

  // Start the WebRTC connection
  async function start() {
      try {
          setStatusMessage('Connecting...', '#ff9');
          Logger.webrtc('Fetching WebRTC configuration');
          
          const configResponse = await fetch('/config');
          if (!configResponse.ok) {
              throw new Error(`Config fetch failed: ${configResponse.status}`);
          }
          const config = await configResponse.json();
          
          config.iceTransportPolicy = 'all';
          
          if (!config.iceServers || !config.iceServers.length) {
              throw new Error('No ICE servers provided');
          }
          const iceServer = config.iceServers[0];
          if (!iceServer.urls || !iceServer.urls.length) {
              throw new Error('No ICE server URLs provided');
          }
          iceServer.urls.forEach(url => {
              if (url.startsWith('turn:') && !url.includes('?transport=')) {
                  Logger.ice('Warning: TURN URL missing transport parameter', url);
              }
              if (url.startsWith('stun:') && url.includes('?transport=')) {
                  Logger.ice('Warning: STUN URL should not have transport parameter', url);
              }
          });
          if (iceServer.urls.some(url => url.startsWith('turn:')) && 
              (!iceServer.username || !iceServer.credential)) {
              throw new Error('Missing TURN credentials');
          }
          
          Logger.ice('Using ICE configuration', {
              urls: iceServer.urls,
              username: iceServer.username,
              credentialProvided: !!iceServer.credential,
              iceTransportPolicy: config.iceTransportPolicy
          });

          window.pc = new RTCPeerConnection(config);
          const sessionId = SessionManager.getSessionId();
          window.monitor = new ConnectionMonitor(window.pc);
          const iceHandler = new ICEHandler(window.pc, sessionId);

          // Add audio transceiver
          window.pc.addTransceiver('audio', {
              direction: 'recvonly'
          });

          window.pc.ontrack = event => {
              Logger.webrtc('Received track', {
                  kind: event.track.kind,
                  id: event.track.id
              });
              if (event.track.kind === 'audio') {
                  window.audioHandler = new AudioHandler(event.track);
              }
          };

          const offer = await window.pc.createOffer();
          await window.pc.setLocalDescription(offer);
          Logger.webrtc('Created and set local description');

          // Send offer
          const offerPayload = {
              sdp: btoa(JSON.stringify({
                  type: offer.type,
                  sdp: offer.sdp
              }))
          };
          const offerResponse = await fetch('/offer', {
              method: 'POST',
              headers: {
                  'Content-Type': 'application/json',
                  'X-Session-ID': sessionId
              },
              body: JSON.stringify(offerPayload)
          });
          if (!offerResponse.ok) {
              throw new Error(`Failed to send offer: ${offerResponse.status}`);
          }
          const encodedAnswer = await offerResponse.json();
          Logger.webrtc('Received answer from server');

          // Decode answer
          const decodedAnswer = JSON.parse(atob(encodedAnswer.sdp));
          const answer = new RTCSessionDescription({
              type: 'answer',
              sdp: decodedAnswer.sdp
          });
          await window.pc.setRemoteDescription(answer);
          iceHandler.setRemoteDescription(true);
          Logger.webrtc('Set remote description');

          // Start stats monitoring
          setInterval(async () => {
              const stats = await window.monitor.getStats();
              Logger.webrtc('Connection stats', stats);
          }, 5000);

          setStatusMessage('Connected', '#9f9');

          // Example: "fake streaming" logs for user feedback
          let chunkCount = 0;
          window.streamingInterval = setInterval(() => {
              if (!window.pc) return; // if closed, stop
              chunkCount++;
              Logger.log('STREAM', `Synth chunk #${chunkCount} streaming...`);
          }, 3000);

      } catch (error) {
          Logger.error('Failed to start', error);
          setStatusMessage('Failed to connect', '#f66');
      }
  }

  // Single toggle for Play/Stop
  function toggleSynth(button) {
      if (!button) return;

      if (button.dataset.state === 'stopped') {
          // Start the connection
          start();
          button.dataset.state = 'playing';
          button.textContent = '■'; // stop symbol
          Logger.log('UI', 'Synth started');
      } else {
          // Stop the connection
          stopSynth();
          button.dataset.state = 'stopped';
          button.textContent = '>'; // play symbol
          Logger.log('UI', 'Synth stopped');

          // Clear fake streaming logs
          if (window.streamingInterval) {
              clearInterval(window.streamingInterval);
              window.streamingInterval = null;
          }
      }
  }
  </script>

</body>
</html>
