// Import styles first
import './styles/main.css';

// Import managers and types
import { AudioManager } from './lib/audio/AudioManager';
import { SessionManager } from './lib/session/SessionManager';
import { AudioVisualizer } from './components/audio-visualizer';
import { PlaybackControls } from './components/playback-controls';
import { CodeViewer } from './components/code-viewer';
import type { AudioStateChangeEvent } from './types/audio';

// Register custom elements if not already registered
const register = (name: string, constructor: CustomElementConstructor) => {
  if (!customElements.get(name)) {
    customElements.define(name, constructor);
  }
};

register('audio-visualizer', AudioVisualizer);
register('playback-controls', PlaybackControls);
register('code-viewer', CodeViewer);

// Initialize components
const audioManager = new AudioManager();
const sessionManager = SessionManager.getInstance();

// Get component references with proper typing
const visualizer = document.querySelector('audio-visualizer') as AudioVisualizer;
const controls = document.querySelector('playback-controls') as PlaybackControls;
const codeViewer = document.querySelector('code-viewer') as CodeViewer;

if (!visualizer || !controls || !codeViewer) {
  throw new Error('Required components not found');
}

// Connect components
controls.setAudioManager(audioManager);
codeViewer.setSessionManager(sessionManager);

// Listen for audio state changes
window.addEventListener('audioStateChange', ((event: AudioStateChangeEvent) => {
  const { connectionStatus } = event.detail;
  
  if (connectionStatus === 'connected') {
    // Only set the analyser after connection is established
    const analyser = audioManager.getAnalyserNode();
    if (analyser) {
      visualizer.setAnalyser(analyser);
    }
    codeViewer.loadCode();
  }
}) as EventListener);

// Mobile optimization
function setupMobileOptimizations() {
  // Prevent bounce scrolling on iOS
  document.body.addEventListener('touchmove', (e) => {
    if (e.target instanceof HTMLElement && !e.target.closest('pre')) {
      e.preventDefault();
    }
  }, { passive: false });

  // Handle iOS audio context unlock
  document.addEventListener('touchstart', () => {
    const AudioContext = window.AudioContext || (window as any).webkitAudioContext;
    if (AudioContext) {
      const context = new AudioContext();
      context.resume().then(() => {
        context.close();
      });
    }
  }, { once: true });

  // Adjust viewport height for mobile browsers
  const vh = window.innerHeight * 0.01;
  document.documentElement.style.setProperty('--vh', `${vh}px`);
  window.addEventListener('resize', () => {
    const vh = window.innerHeight * 0.01;
    document.documentElement.style.setProperty('--vh', `${vh}px`);
  });
}

setupMobileOptimizations(); 
