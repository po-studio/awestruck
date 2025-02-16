// Temporarily commenting out styles
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
const statusElement = document.querySelector('.connection-status') as HTMLDivElement;

if (!visualizer || !controls || !codeViewer || !statusElement) {
  throw new Error('Required components not found');
}

// Connect components
controls.setAudioManager(audioManager);
codeViewer.setSessionManager(sessionManager);

// Update connection status display
function updateConnectionStatus(status: string) {
  const statusMap: Record<string, { text: string }> = {
    'disconnected': { text: 'offline' },
    'connecting': { text: 'connecting' },
    'connected': { text: 'live' },
    'stopping': { text: 'stopping' }
  };

  const { text } = statusMap[status] || statusMap['disconnected'];
  statusElement.textContent = text;
  
  // Remove any existing status classes
  statusElement.classList.remove('status-disconnected', 'status-connecting', 'status-connected', 'status-stopping');
  // Add new status class
  statusElement.classList.add(`status-${status}`);
}

// Listen for audio state changes
window.addEventListener('audioStateChange', ((event: AudioStateChangeEvent) => {
  const { connectionStatus } = event.detail;
  updateConnectionStatus(connectionStatus);
  
  if (connectionStatus === 'connected') {
    const analyser = audioManager.getAnalyserNode();
    if (analyser) {
      visualizer.setAnalyser(analyser);
      codeViewer.show();
      codeViewer.loadCode();
    }
  } else if (['disconnected', 'stopping'].includes(connectionStatus)) {
    // Hide code viewer when stopping or disconnected
    codeViewer.hide();
  }
}) as EventListener);

// Set initial status
updateConnectionStatus('disconnected');

// Mobile optimization
function setupMobileOptimizations() {
  // Prevent double-tap zoom on buttons
  document.querySelectorAll('button').forEach(button => {
    button.addEventListener('touchend', (e) => {
      e.preventDefault();
    }, { passive: false });
  });

  // Prevent pull-to-refresh
  document.body.style.overscrollBehavior = 'none';

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

  // Adjust viewport height for mobile browsers (iOS Safari fix)
  function updateViewportHeight() {
    const vh = window.innerHeight * 0.01;
    document.documentElement.style.setProperty('--vh', `${vh}px`);
  }
  
  window.addEventListener('resize', updateViewportHeight);
  window.addEventListener('orientationchange', updateViewportHeight);
  updateViewportHeight();
}

setupMobileOptimizations(); 
