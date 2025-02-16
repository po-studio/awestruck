import type { AudioState, AudioVisualizerOptions, WebRTCConfig } from '../../types/audio';
import { SessionManager } from '../session/SessionManager';

// logging helper to keep logs consistent and filterable
const log = (context: string, message: string, data?: any) => {
  const timestamp = new Date().toISOString();
  const prefix = `[AudioManager][${context}]`;
  console.log(`${prefix} ${message}`, data ? data : '');
};

export class AudioManager {
  private context?: AudioContext;
  private analyser?: AnalyserNode;
  private gainNode?: GainNode;
  private state: AudioState;
  private options: AudioVisualizerOptions;
  private peerConnection?: RTCPeerConnection;
  private sessionManager: SessionManager;
  private pendingCandidates: RTCIceCandidate[] = [];
  private remoteDescriptionSet: boolean = false;
  private audioElement?: HTMLAudioElement;

  constructor(options?: Partial<AudioVisualizerOptions>) {
    log('constructor', 'Initializing AudioManager');
    this.sessionManager = SessionManager.getInstance();
    
    this.options = {
      fftSize: 2048,
      smoothingTimeConstant: 0.85,
      minDecibels: -90,
      maxDecibels: -10,
      ...options
    };

    log('constructor', 'AudioManager options:', this.options);

    this.state = {
      isPlaying: false,
      volume: 1.0,
      connectionStatus: 'disconnected'
    };
  }

  private setupAudioElement(track: MediaStreamTrack): void {
    log('setupAudioElement', 'Setting up audio element');
    
    const stream = new MediaStream([track]);
    this.audioElement = new Audio();
    this.audioElement.autoplay = true;
    this.audioElement.controls = true;
    this.audioElement.style.display = 'none';
    this.audioElement.volume = 1.0;
    document.body.appendChild(this.audioElement);
    
    // Add error handling and state monitoring
    this.audioElement.addEventListener('error', (e) => {
      log('audioElement', 'Playback error', (e.target as HTMLAudioElement).error);
    });
    
    this.audioElement.addEventListener('play', () => {
      log('audioElement', 'Playback started');
    });
    
    this.audioElement.addEventListener('canplay', () => {
      log('audioElement', 'Can start playing');
      if (this.context?.state === 'suspended') {
        this.context.resume().then(() => {
          log('audioElement', 'Resumed audio context');
        });
      }
    });
    
    this.audioElement.srcObject = stream;
    
    // Start playback
    this.audioElement.play().catch(err => {
      log('audioElement', 'Playback failed', err);
      // Try to recover by requesting user interaction
      document.body.addEventListener('click', () => {
        this.audioElement?.play().catch(e => log('audioElement', 'Retry playback failed', e));
      }, { once: true });
    });
  }

  private async waitForICEConnection(timeout: number = 30000): Promise<void> {
    return new Promise((resolve, reject) => {
      if (!this.peerConnection) {
        reject(new Error('No peer connection'));
        return;
      }

      const timer = setTimeout(() => {
        reject(new Error('ICE connection timeout'));
      }, timeout);

      const checkState = () => {
        if (!this.peerConnection) return;
        
        const state = this.peerConnection.iceConnectionState;
        if (state === 'connected' || state === 'completed') {
          clearTimeout(timer);
          resolve();
        } else if (state === 'failed' || state === 'disconnected' || state === 'closed') {
          clearTimeout(timer);
          reject(new Error(`ICE connection failed: ${state}`));
        }
      };

      this.peerConnection.addEventListener('iceconnectionstatechange', checkState);
      checkState();
    });
  }

  private async initializeAudioContext(): Promise<void> {
    log('initializeAudioContext', 'Starting audio context initialization');
    if (!this.context) {
      this.context = new AudioContext();
      this.analyser = this.context.createAnalyser();
      this.gainNode = this.context.createGain();
      Object.assign(this.analyser, this.options);
      this.gainNode.connect(this.analyser);
      this.gainNode.connect(this.context.destination);
      log('initializeAudioContext', 'Created new AudioContext and nodes', {
        contextState: this.context.state,
        analyserProps: {
          fftSize: this.analyser.fftSize,
          smoothingTimeConstant: this.analyser.smoothingTimeConstant
        }
      });
    }

    if (this.context.state === 'suspended') {
      log('initializeAudioContext', 'Resuming suspended audio context');
      await this.context.resume();
      log('initializeAudioContext', 'Audio context resumed successfully');
    }
  }

  public async connect(): Promise<void> {
    try {
      log('connect', 'Starting connection process');
      await this.initializeAudioContext();
      this.setState({ connectionStatus: 'connecting' });
      
      log('connect', 'Fetching WebRTC configuration');
      const configResponse = await fetch('/config');
      if (!configResponse.ok) {
        throw new Error(`Config fetch failed: ${configResponse.status}`);
      }
      const config: WebRTCConfig = await configResponse.json();
      log('connect', 'Received WebRTC config:', config);

      // Allow all ICE transport policies
      config.iceTransportPolicy = 'all';

      await this.setupWebRTC(config);
      
      // Wait for ICE connection
      await this.waitForICEConnection();
      
      this.setState({ connectionStatus: 'connected' });
      log('connect', 'Connection established successfully');
    } catch (error) {
      log('connect', 'Connection failed', error);
      this.setState({ connectionStatus: 'disconnected' });
      throw error;
    }
  }

  private async setupWebRTC(config: WebRTCConfig): Promise<void> {
    log('setupWebRTC', 'Setting up WebRTC connection', config);
    
    this.peerConnection = new RTCPeerConnection(config);
    
    // Add audio transceiver
    const transceiver = this.peerConnection.addTransceiver('audio', { direction: 'recvonly' });
    log('setupWebRTC', 'Added audio transceiver', {
      direction: transceiver.direction,
      currentDirection: transceiver.currentDirection
    });

    // Handle incoming tracks
    this.peerConnection.ontrack = (event) => {
      log('ontrack', 'Received media track', {
        kind: event.track.kind,
        id: event.track.id,
        label: event.track.label
      });
      
      if (event.track.kind === 'audio' && this.context && this.gainNode) {
        // Set up both Audio element and AudioContext
        this.setupAudioElement(event.track);
        
        const source = this.context.createMediaStreamSource(
          new MediaStream([event.track])
        );
        source.connect(this.gainNode);
        log('ontrack', 'Connected audio track to gain node');
      }
    };

    // Add connection state change logging
    this.peerConnection.onconnectionstatechange = () => {
      log('connectionState', 'Connection state changed', {
        state: this.peerConnection?.connectionState,
        iceState: this.peerConnection?.iceConnectionState,
        iceGatheringState: this.peerConnection?.iceGatheringState
      });
    };

    this.peerConnection.oniceconnectionstatechange = () => {
      log('iceConnectionState', 'ICE connection state changed', {
        state: this.peerConnection?.iceConnectionState,
        selectedPair: this.peerConnection?.getStats()
      });
    };

    this.peerConnection.onicegatheringstatechange = () => {
      log('iceGatheringState', 'ICE gathering state changed', {
        state: this.peerConnection?.iceGatheringState
      });
    };

    this.peerConnection.onicecandidate = async (event) => {
      if (event.candidate) {
        log('iceCandidate', 'New ICE candidate', {
          candidate: event.candidate.candidate,
          type: event.candidate.type,
          protocol: event.candidate.protocol,
          address: event.candidate.address,
          port: event.candidate.port,
          priority: event.candidate.priority
        });

        if (!this.remoteDescriptionSet) {
          log('iceCandidate', 'Queuing candidate until remote description is set');
          this.pendingCandidates.push(event.candidate);
          return;
        }

        try {
          await this.sendICECandidate(event.candidate);
        } catch (error) {
          log('iceCandidate', 'Failed to send candidate, queuing', error);
          this.pendingCandidates.push(event.candidate);
        }
      }
    };

    // Create and set local description
    log('setupWebRTC', 'Creating offer');
    const offer = await this.peerConnection.createOffer();
    log('setupWebRTC', 'Setting local description', offer);
    await this.peerConnection.setLocalDescription(offer);

    // Send offer to server
    const offerPayload = {
      sdp: btoa(JSON.stringify({
        type: offer.type,
        sdp: offer.sdp
      }))
    };

    log('setupWebRTC', 'Sending offer to server');
    const offerResponse = await fetch('/offer', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': this.sessionManager.getSessionId()
      },
      body: JSON.stringify(offerPayload)
    });

    if (!offerResponse.ok) {
      const error = `Failed to send offer: ${offerResponse.status}`;
      log('setupWebRTC', error);
      throw new Error(error);
    }

    // Handle answer
    log('setupWebRTC', 'Processing server answer');
    const encodedAnswer = await offerResponse.json();
    const decodedAnswer = JSON.parse(atob(encodedAnswer.sdp));
    const answer = new RTCSessionDescription({
      type: 'answer',
      sdp: decodedAnswer.sdp
    });

    log('setupWebRTC', 'Setting remote description', answer);
    await this.peerConnection.setRemoteDescription(answer);
    this.remoteDescriptionSet = true;
    log('setupWebRTC', 'WebRTC setup completed successfully');

    // Send any pending candidates
    if (this.pendingCandidates.length > 0) {
      log('setupWebRTC', `Sending ${this.pendingCandidates.length} pending candidates`);
      for (const candidate of this.pendingCandidates) {
        try {
          await this.sendICECandidate(candidate);
        } catch (error) {
          log('setupWebRTC', 'Failed to send pending candidate', error);
        }
      }
      this.pendingCandidates = [];
    }
  }

  private async sendICECandidate(candidate: RTCIceCandidate): Promise<void> {
    const candidateObj = {
      candidate: candidate.candidate,
      sdpMid: candidate.sdpMid,
      sdpMLineIndex: candidate.sdpMLineIndex,
      usernameFragment: candidate.usernameFragment
    };

    const response = await fetch('/ice-candidate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': this.sessionManager.getSessionId()
      },
      body: JSON.stringify({
        candidate: btoa(JSON.stringify(candidateObj))
      })
    });

    if (!response.ok) {
      throw new Error(`Failed to send ICE candidate: ${response.status}`);
    }
  }

  public async disconnect(): Promise<void> {
    try {
      log('disconnect', 'Starting disconnect process');
      await fetch('/stop', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Session-ID': this.sessionManager.getSessionId()
        }
      });

      if (this.peerConnection) {
        log('disconnect', 'Closing peer connection');
        this.peerConnection.close();
        this.peerConnection = undefined;
      }

      if (this.audioElement) {
        this.audioElement.pause();
        this.audioElement.srcObject = null;
        this.audioElement.remove();
        this.audioElement = undefined;
      }

      this.setState({ 
        isPlaying: false,
        connectionStatus: 'disconnected' 
      });
      log('disconnect', 'Disconnect completed successfully');
    } catch (error) {
      log('disconnect', 'Error during disconnect', error);
      throw error;
    }
  }

  public setVolume(value: number): void {
    log('setVolume', `Setting volume to ${value}`);
    if (this.gainNode) {
      this.gainNode.gain.value = value;
    }
    if (this.audioElement) {
      this.audioElement.volume = value;
    }
    this.setState({ volume: value });
  }

  public getAnalyserNode(): AnalyserNode | undefined {
    return this.analyser;
  }

  private setState(newState: Partial<AudioState>): void {
    this.state = { ...this.state, ...newState };
    log('setState', 'State updated', this.state);
    window.dispatchEvent(new CustomEvent('audioStateChange', {
      detail: this.state
    }));
  }
} 