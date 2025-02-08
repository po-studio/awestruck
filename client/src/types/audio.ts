export interface AudioState {
  isPlaying: boolean;
  volume: number;
  connectionStatus: 'disconnected' | 'connecting' | 'connected';
}

export interface AudioVisualizerOptions {
  fftSize: number;
  smoothingTimeConstant: number;
  minDecibels: number;
  maxDecibels: number;
}

export interface WebRTCConfig {
  iceServers: RTCIceServer[];
  iceTransportPolicy?: RTCIceTransportPolicy;
}

export interface SessionManager {
  getSessionId(): string;
}

// Custom events
export interface AudioStateChangeEvent extends CustomEvent {
  detail: AudioState;
}

export interface PlaybackToggleEvent extends CustomEvent {
  detail: {
    isPlaying: boolean;
  };
} 