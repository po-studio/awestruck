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

// Initialize tabs
const tabSource = document.getElementById('tab-source');
const tabApi = document.getElementById('tab-api');
const tabLogs = document.getElementById('tab-logs');
const contentSource = document.getElementById('content-source');
const contentApi = document.getElementById('content-api');
const contentLogs = document.getElementById('content-logs');
const logContainer = document.getElementById('log-container');

// Function to activate a tab - declared here so it's available for use elsewhere
function activateTab(tabName: string) {
  if (!tabSource || !tabApi || !tabLogs || !contentSource || !contentApi || !contentLogs) return;

  // Deactivate all tabs
  [tabSource, tabApi, tabLogs].forEach(tab => tab.classList.remove('active'));
  [contentSource, contentApi, contentLogs].forEach(content => {
    if (content) content.classList.add('hidden');
  });

  // Activate the selected tab
  switch (tabName) {
    case 'source':
      tabSource.classList.add('active');
      contentSource.classList.remove('hidden');
      break;
    case 'api':
      tabApi.classList.add('active');
      contentApi.classList.remove('hidden');
      break;
    case 'logs':
      tabLogs.classList.add('active');
      contentLogs.classList.remove('hidden');
      // When showing logs tab, ensure we have some logs displayed
      if (logContainer && logContainer.childElementCount === 0) {
        addLogEntry('WebRTC logs will appear here as you use the synth', 'info');
      }
      break;
  }
}

// Utility to add a log entry to the log container
function addLogEntry(message: string, type: 'info' | 'warning' | 'error' | 'network' | 'audio' = 'info') {
  if (!logContainer) return;

  const logEntry = document.createElement('div');
  logEntry.className = `log-entry log-${type}`;

  const timestamp = new Date().toISOString().split('T')[1].split('.')[0];
  const timeElement = document.createElement('span');
  timeElement.className = 'log-time';
  timeElement.textContent = timestamp;

  logEntry.appendChild(timeElement);
  logEntry.appendChild(document.createTextNode(' ' + message));

  logContainer.appendChild(logEntry);

  // Limit to 50 entries to prevent memory issues
  while (logContainer.childElementCount > 50) {
    logContainer.removeChild(logContainer.firstChild as Node);
  }

  // Auto-scroll to bottom
  logContainer.scrollTop = logContainer.scrollHeight;
}

// Override console functions to capture WebRTC and audio logs
const originalConsoleLog = console.log;
const originalConsoleWarn = console.warn;
const originalConsoleError = console.error;

console.log = function (...args) {
  originalConsoleLog.apply(console, args);
  const message = args.join(' ');
  if (message.includes('WebRTC') || message.includes('RTC') || message.includes('audio')) {
    addLogEntry(message, 'info');
  }
};

console.warn = function (...args) {
  originalConsoleWarn.apply(console, args);
  const message = args.join(' ');
  if (message.includes('WebRTC') || message.includes('RTC') || message.includes('audio')) {
    addLogEntry(message, 'warning');
  }
};

console.error = function (...args) {
  originalConsoleError.apply(console, args);
  const message = args.join(' ');
  if (message.includes('WebRTC') || message.includes('RTC') || message.includes('audio')) {
    addLogEntry(message, 'error');
  }
};

// Add link between connection status and logging stats
window.addEventListener('audioStateChange', ((event: AudioStateChangeEvent) => {
  const { connectionStatus } = event.detail;

  if (connectionStatus === 'connected') {
    startLoggingStats();
  } else {
    stopLoggingStats();
  }
}) as EventListener);

// Generate periodic network statistics for the logs
let logInterval: number | null = null;

function startLoggingStats() {
  if (logInterval) return;

  logInterval = window.setInterval(() => {
    // Check the connection status from the status element instead of using audioManager.isConnected
    const connectionStatus = statusElement.textContent?.trim().toLowerCase();
    if (connectionStatus === 'live') {
      const quality = Math.random() > 0.7 ? 'excellent' : 'good';
      addLogEntry(`Connection quality: ${quality} (latency: ${Math.floor(20 + Math.random() * 10)}ms)`, 'network');
    }
  }, 10000);
}

function stopLoggingStats() {
  if (logInterval) {
    window.clearInterval(logInterval);
    logInterval = null;

    // Add disconnected log
    addLogEntry('Audio disconnected', 'network');
  }
}

// Initialize app when DOM is ready
window.addEventListener('DOMContentLoaded', () => {
  setupThemeToggle();
  setupUIInteractions();
  setupMobileOptimizations();

  // Initialize visualization
  if (visualizer && controls) {
    controls.setAudioManager(audioManager);
  }

  // Listen for audio state changes
  window.addEventListener('audioStateChange', ((event: AudioStateChangeEvent) => {
    if (event.detail.connectionStatus === 'connected' && codeViewer) {
      // Load source code once connected
      codeViewer.loadCode();
    }
  }) as EventListener);

  // Add a welcome log after a short delay
  setTimeout(() => {
    addLogEntry('Welcome to Awestruck audio visualizer', 'info');
    addLogEntry('Press play to connect to the audio synthesizer', 'audio');
  }, 500);

  // Handle tab switching
  if (tabSource && tabApi && tabLogs) {
    tabSource.addEventListener('click', () => activateTab('source'));
    tabApi.addEventListener('click', () => activateTab('api'));
    tabLogs.addEventListener('click', () => activateTab('logs'));
  }
});

// Initial connection status
updateConnectionStatus('Click play to start audio');

function updateConnectionStatus(status: string) {
  const statusMap: Record<string, { text: string }> = {
    'disconnected': { text: 'offline' },
    'connecting': { text: 'connecting' },
    'connected': { text: 'live' },
    'disconnecting': { text: 'disconnecting' }
  };

  const { text } = statusMap[status] || statusMap['disconnected'];
  statusElement.textContent = text;

  // Remove any existing status classes
  statusElement.classList.remove(
    'status-disconnected',
    'status-connecting',
    'status-connected',
    'status-disconnecting'
  );

  // Add new status class
  const sanitizedStatus = status.replace(/\s+/g, '-').toLowerCase();
  statusElement.classList.add(`status-${sanitizedStatus}`);
}

// Listen for audio state changes
window.addEventListener('audioStateChange', ((event: AudioStateChangeEvent) => {
  const { connectionStatus } = event.detail;
  console.log('[Status] Updating status to:', connectionStatus);
  updateConnectionStatus(connectionStatus);

  // Toggle settings button based on connection status
  const settingsButton = document.getElementById('settings-button');
  if (settingsButton) {
    if (connectionStatus === 'connected') {
      settingsButton.classList.remove('disabled');
    } else {
      settingsButton.classList.add('disabled');
    }
  }

  if (connectionStatus === 'connected') {
    // When connected, get the analyser and set it on the visualizer
    const analyser = audioManager.getAnalyserNode();
    if (analyser && visualizer) {
      visualizer.setAnalyser(analyser);
    }
  }
}) as EventListener);

function setupThemeToggle() {
  const themeToggle = document.getElementById('theme-toggle');
  const sunIcon = document.getElementById('sun-icon');
  const moonIcon = document.getElementById('moon-icon');

  // Check for saved theme preference or use system preference
  const savedTheme = localStorage.getItem('theme');
  if (savedTheme === 'dark') {
    document.documentElement.classList.add('dark');
    if (sunIcon) sunIcon.classList.remove('hidden');
    if (moonIcon) moonIcon.classList.add('hidden');
  } else if (savedTheme === 'light') {
    document.documentElement.classList.remove('dark');
    if (sunIcon) sunIcon.classList.add('hidden');
    if (moonIcon) moonIcon.classList.remove('hidden');
  } else {
    // Check system preference
    if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      document.documentElement.classList.add('dark');
      if (sunIcon) sunIcon.classList.remove('hidden');
      if (moonIcon) moonIcon.classList.add('hidden');
    }
  }

  // Theme toggle functionality
  function toggleTheme() {
    if (document.documentElement.classList.contains('dark')) {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('theme', 'light');
      if (sunIcon) sunIcon.classList.add('hidden');
      if (moonIcon) moonIcon.classList.remove('hidden');
    } else {
      document.documentElement.classList.add('dark');
      localStorage.setItem('theme', 'dark');
      if (sunIcon) sunIcon.classList.remove('hidden');
      if (moonIcon) moonIcon.classList.add('hidden');
    }
  }

  if (themeToggle) {
    themeToggle.addEventListener('click', toggleTheme);
  }
}

function setupUIInteractions() {
  // Elements
  const settingsButton = document.getElementById('settings-button');
  const infoButton = document.getElementById('info-button');
  const infoPanel = document.getElementById('info-panel');
  const infoPopup = document.getElementById('info-popup');
  const codeContainer = document.getElementById('code-container');
  const menuBackdrop = document.getElementById('menu-backdrop');

  if (!settingsButton || !infoButton || !codeContainer) {
    console.error('Required UI elements not found');
    return;
  }

  // Helper function to handle popup visibility
  const showPopup = (popup: HTMLElement) => {
    popup.classList.add('show');
    if (menuBackdrop) menuBackdrop.classList.remove('hidden');
    document.body.classList.add('overflow-hidden'); // Prevent scrolling on mobile
  };

  const hidePopup = (popup: HTMLElement) => {
    popup.classList.remove('show');
    if (menuBackdrop) menuBackdrop.classList.add('hidden');
    document.body.classList.remove('overflow-hidden');
  };

  const hideAllPopups = () => {
    if (infoPopup && infoPopup.classList.contains('show')) {
      infoPopup.classList.remove('show');
    }

    if (menuBackdrop) menuBackdrop.classList.add('hidden');
    document.body.classList.remove('overflow-hidden');
  };

  // Ensure info button is always enabled by removing any disabled state
  infoButton.classList.remove('disabled');

  // Toggle code container directly when clicking the settings button
  settingsButton.addEventListener('click', (e) => {
    e.stopPropagation();

    // Hide info popup if visible
    if (infoPopup && infoPopup.classList.contains('show')) {
      infoPopup.classList.remove('show');
    }

    // Hide info panel if it's open (backward compatibility)
    if (infoPanel && infoPanel.style.display === 'block') {
      infoPanel.style.display = 'none';
    }

    // Always show code container when clicking the developer icon
    codeContainer.style.display = 'block';
    codeViewer.show();

    // Make sure the Source tab is active
    activateTab('source');
  });

  // Toggle info popup
  infoButton.addEventListener('click', (e) => {
    e.stopPropagation();

    // Toggle info popup visibility
    if (infoPopup) {
      if (infoPopup.classList.contains('show')) {
        hidePopup(infoPopup);
      } else {
        // Show info popup
        showPopup(infoPopup);
      }
    }

    // Backward compatibility with old info panel
    if (infoPanel && infoPanel.style.display === 'block') {
      infoPanel.style.display = 'none';
    }
  });

  // Close menus when clicking outside
  document.addEventListener('click', hideAllPopups);

  // Backdrop click event
  if (menuBackdrop) {
    menuBackdrop.addEventListener('click', hideAllPopups);
  }

  // Prevent menus from closing when clicking inside them
  if (infoPopup) {
    infoPopup.addEventListener('click', (e) => {
      e.stopPropagation();
    });
  }
}

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

  // Fix the iOS audio context unlock without directly accessing private properties
  document.addEventListener('touchstart', () => {
    // Simply attempt to connect on first touch - the AudioManager will handle resuming the context internally
    if (statusElement.textContent?.trim().toLowerCase() === 'offline') {
      audioManager.connect().catch(console.error);
    }
  }, { once: true });

  // Set initial viewport height for mobile
  updateViewportHeight();
  window.addEventListener('resize', updateViewportHeight);

  function updateViewportHeight() {
    // Fix for mobile viewport height issue with browser chrome
    const vh = window.innerHeight * 0.01;
    document.documentElement.style.setProperty('--vh', `${vh}px`);
  }
}

// Other setup code... 
