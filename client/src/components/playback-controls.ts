// handles play/stop button and volume controls
export class PlaybackControls extends HTMLElement {
  private audioManager?: any; // Will be set via public method

  constructor() {
    super();
    this.setupComponent();
    this.setupKeyboardControls();
  }

  private setupComponent(): void {
    const shadow = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');

    style.textContent = `
      :host {
        display: flex;
        align-items: center;
        position: relative;
        z-index: 10;
      }

      .container {
        display: flex;
        align-items: center;
        gap: 0rem;
      }

      .button-container {
        position: relative;
        width: 42px;
        height: 42px;
      }

      button {
        position: absolute;
        inset: 0;
        background: rgba(40, 40, 40, 0.5);
        border: none;
        border-radius: 10px;
        padding: 0;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        color: #ffffff;
        opacity: 0.9;
        transition: all 0.2s ease;
        backdrop-filter: blur(4px);
      }

      button:not(:disabled):hover {
        opacity: 1;
        transform: scale(1.02);
        background: rgba(60, 60, 60, 0.55);
      }

      button:disabled {
        opacity: 0.5;
        cursor: default;
      }

      /* Improved pulse animation */
      @keyframes pulse {
        0% { transform: scale(1); opacity: 0.9; }
        50% { transform: scale(1.03); opacity: 1; }
        100% { transform: scale(1); opacity: 0.9; }
      }

      button.initial {
        animation: pulse 2.5s infinite cubic-bezier(0.25, 0.46, 0.45, 0.94);
      }

      button.initial:hover {
        animation-play-state: paused;
      }

      /* Loading spinner */
      @keyframes spin {
        to { transform: rotate(360deg); }
      }

      .spinner {
        display: none;
        position: absolute;
        inset: 0;
        border: 1.5px solid rgba(255,255,255,0.1);
        border-top-color: rgba(255,255,255,0.7);
        border-radius: 50%;
        animation: spin 1s linear infinite;
      }

      :host([loading]) .spinner {
        display: block;
      }

      :host([loading]) button {
        opacity: 0;
      }

      /* SVG icons */
      svg {
        width: 20px;
        height: 20px;
        stroke: currentColor;
        stroke-width: 1.25;
        fill: none;
      }
    `;

    const template = document.createElement('template');
    template.innerHTML = `
      <div class="container">
        <div class="button-container">
          <button class="initial" aria-label="Play/Pause" data-state="stopped">
            <svg viewBox="0 0 24 24">
              <polygon points="7 4 21 12 7 20 7 4"></polygon>
            </svg>
          </button>
          <div class="spinner"></div>
        </div>
      </div>
    `;

    shadow.appendChild(style);
    shadow.appendChild(template.content.cloneNode(true));

    const button = shadow.querySelector('button');
    if (button) {
      button.addEventListener('click', () => this.togglePlayback());
    }
  }

  private async togglePlayback(): Promise<void> {
    if (!this.audioManager) return;

    const button = this.shadowRoot?.querySelector('button');

    if (!button) return;

    const isPlaying = button.getAttribute('data-state') === 'playing';

    if (!isPlaying) {
      // Remove initial pulse animation
      button.classList.remove('initial');

      // Disable button and show loading state
      button.disabled = true;
      this.setAttribute('loading', '');

      try {
        await this.audioManager.connect();
        button.setAttribute('data-state', 'playing');
        button.innerHTML = this.getStopIcon();
      } catch (error) {
        console.error('Playback toggle failed:', error);
        button.setAttribute('data-state', 'stopped');
        button.innerHTML = this.getPlayIcon();
      } finally {
        // Re-enable button and hide loading state
        button.disabled = false;
        this.removeAttribute('loading');
      }
    } else {
      button.disabled = true;
      this.setAttribute('loading', '');

      try {
        await this.audioManager.disconnect();
        button.setAttribute('data-state', 'stopped');
        button.innerHTML = this.getPlayIcon();
        // Add initial pulse animation back when stopped
        button.classList.add('initial');
      } catch (error) {
        console.error('Playback toggle failed:', error);
      } finally {
        button.disabled = false;
        this.removeAttribute('loading');
      }
    }
  }

  private getPlayIcon(): string {
    return `
      <svg viewBox="0 0 24 24">
        <polygon points="7 4 21 12 7 20 7 4"></polygon>
      </svg>
    `;
  }

  private getStopIcon(): string {
    return `
      <svg viewBox="0 0 24 24">
        <line x1="9" y1="6" x2="9" y2="18"></line>
        <line x1="15" y1="6" x2="15" y2="18"></line>
      </svg>
    `;
  }

  public setAudioManager(manager: any): void {
    this.audioManager = manager;
  }

  private setupKeyboardControls(): void {
    // Handle spacebar press
    document.addEventListener('keydown', (e) => {
      // Only handle spacebar and prevent default space behavior
      if (e.code === 'Space' || e.key === ' ') {
        e.preventDefault();

        // Don't trigger if user is typing in an input/textarea
        if (e.target instanceof HTMLElement) {
          const tag = e.target.tagName.toLowerCase();
          if (tag === 'input' || tag === 'textarea') {
            return;
          }
        }

        // Find and click the button
        const button = this.shadowRoot?.querySelector('button');
        if (button && !button.disabled) {
          button.click();
        }
      }
    });
  }
}

customElements.define('playback-controls', PlaybackControls); 