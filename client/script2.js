// why we need session management:
// - maintains consistent identity across reconnects
// - enables proper turn credential handling
// - helps with debugging and monitoring
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

// why we need structured logging:
// - consistent log format
// - easier to filter and analyze
// - better debugging experience
class Logger {
    static log(type, message, data) {
        const entry = {
            time: new Date().toISOString(),
            type,
            message,
            ...(data && { data })
        };
        
        console.log(`[${entry.time}][${type}] ${message}`, data || '');
        
        const logsElement = document.getElementById('logs');
        if (logsElement) {
            logsElement.innerHTML += `[${entry.time}][${type}] ${message} ${data ? JSON.stringify(data, null, 2) : ''}\n`;
            logsElement.scrollTop = logsElement.scrollHeight;
        }
    }

    static webrtc(msg, data) { this.log('WebRTC', msg, data); }
    static ice(msg, data) { this.log('ICE', msg, data); }
    static turn(msg, data) { this.log('TURN', msg, data); }
    static error(msg, data) { this.log('ERROR', msg, data); }
}

// why we need connection state tracking:
// - monitors ice and peer connection state
// - updates ui status
// - provides debugging info
class ConnectionMonitor {
    constructor(pc) {
        this.pc = pc;
        this.startTime = Date.now();
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

// why we need audio handling:
// - sets up audio playback
// - monitors audio levels
// - provides debugging info
class AudioHandler {
    constructor(track) {
        this.track = track;
        this.setupAudioElement();
        this.setupAudioContext();
    }

    setupAudioElement() {
        const audio = new Audio();
        audio.srcObject = new MediaStream([this.track]);
        audio.volume = 1.0; // Ensure volume is up
        audio.autoplay = true; // Ensure autoplay
        
        // Add error handling
        audio.onerror = (err) => {
            Logger.error('Audio playback error', err);
        };
        
        // Log when audio actually starts playing
        audio.onplay = () => {
            Logger.log('AUDIO', 'Playback started');
        };
        
        // Store reference
        this.audioElement = audio;
        
        // Start playback
        audio.play().catch(err => {
            Logger.error('Audio playback failed', err);
            // Try to recover by requesting user interaction
            document.body.addEventListener('click', () => {
                audio.play().catch(err => Logger.error('Retry playback failed', err));
            }, { once: true });
        });
    }

    setupAudioContext() {
        try {
            const ctx = new (window.AudioContext || window.webkitAudioContext)();
            const source = ctx.createMediaStreamSource(new MediaStream([this.track]));
            const gainNode = ctx.createGain();
            const analyser = ctx.createAnalyser();
            
            // Set gain to ensure audio is audible
            gainNode.gain.value = 1.0;
            
            // Connect nodes
            source.connect(gainNode);
            gainNode.connect(analyser);
            gainNode.connect(ctx.destination);
            
            // Store references
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
                peak: Math.max(...dataArray.map(Math.abs)).toFixed(4),
                contextState: this.audioContext?.state,
                gainValue: this.gainNode?.gain.value
            });

            // If we see zero levels, try to resume the context
            if (level === 0 && this.audioContext?.state === 'suspended') {
                this.audioContext.resume();
            }
        };

        setInterval(check, 1000);
    }

    // Add method to adjust volume
    setVolume(value) {
        if (this.audioElement) {
            this.audioElement.volume = value;
        }
        if (this.gainNode) {
            this.gainNode.gain.value = value;
        }
    }
}

// why we need ice candidate handling:
// - ensures proper turn relay allocation
// - manages candidate gathering and sending
// - handles turn server authentication
class ICEHandler {
    constructor(pc, sessionId) {
        this.pc = pc;
        this.sessionId = sessionId;
        this.pendingCandidates = [];
        this.hasRemoteDescription = false;
        this.setupICEHandling();
    }

    setupICEHandling() {
        // Track ICE gathering progress
        this.pc.onicegatheringstatechange = () => {
            Logger.ice('Gathering state changed', {
                state: this.pc.iceGatheringState
            });
            
            if (this.pc.iceGatheringState === 'complete') {
                Logger.ice('Gathering completed');
            }
        };

        // Handle new ICE candidates
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
            // Queue for retry
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

// why we need webrtc setup:
// - initializes peer connection
// - handles signaling
// - manages media streams
async function start() {
    try {
        Logger.webrtc('Fetching WebRTC configuration');
        const configResponse = await fetch('/config');
        if (!configResponse.ok) {
            throw new Error(`Config fetch failed: ${configResponse.status}`);
        }
        const config = await configResponse.json();
        
        // why we need proper ice configuration:
        // - allows both stun and turn
        // - validates url formats
        // - ensures connectivity options
        config.iceTransportPolicy = 'all';
        
        // Validate ICE configuration
        if (!config.iceServers || !config.iceServers.length) {
            throw new Error('No ICE servers provided');
        }
        
        const iceServer = config.iceServers[0];
        if (!iceServer.urls || !iceServer.urls.length) {
            throw new Error('No ICE server URLs provided');
        }
        
        // Validate URL formats
        iceServer.urls.forEach(url => {
            if (url.startsWith('turn:') && !url.includes('?transport=')) {
                Logger.ice('Warning: TURN URL missing transport parameter', url);
            }
            if (url.startsWith('stun:') && url.includes('?transport=')) {
                Logger.ice('Warning: STUN URL should not have transport parameter', url);
            }
        });

        // Only require credentials for TURN
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

        const pc = new RTCPeerConnection(config);
        const sessionId = SessionManager.getSessionId();
        const monitor = new ConnectionMonitor(pc);
        const iceHandler = new ICEHandler(pc, sessionId);

        // Add audio transceiver
        pc.addTransceiver('audio', {
            direction: 'recvonly'
        });

        pc.ontrack = event => {
            Logger.webrtc('Received track', {
                kind: event.track.kind,
                id: event.track.id
            });
            
            if (event.track.kind === 'audio') {
                new AudioHandler(event.track);
            }
        };

        // Create and send offer
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        Logger.webrtc('Created and set local description');

        // Send offer to server
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

        // Decode the base64 answer
        const decodedAnswer = JSON.parse(atob(encodedAnswer.sdp));
        const answer = new RTCSessionDescription({
            type: 'answer',
            sdp: decodedAnswer.sdp
        });

        await pc.setRemoteDescription(answer);
        iceHandler.setRemoteDescription(true);
        Logger.webrtc('Set remote description');
        
        // Start stats monitoring
        setInterval(async () => {
            const stats = await monitor.getStats();
            Logger.webrtc('Connection stats', stats);
        }, 5000);

    } catch (err) {
        Logger.error('Start failed', err);
        throw err;
    }
}

// Setup UI handlers
document.getElementById('startButton').onclick = () => {
    start().catch(err => {
        Logger.error('Fatal error', err);
        document.getElementById('status').textContent = 'Failed';
        document.getElementById('status').style.backgroundColor = '#f66';
    });
};