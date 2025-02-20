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
      }

      .container {
        display: flex;
        align-items: center;
        gap: 0rem;
      }

      .button-container {
        position: relative;
        width: 24px;
        height: 24px;
      }

      button {
        position: absolute;
        inset: 0;
        background: none;
        border: none;
        padding: 0;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        color: #ffffff;
        opacity: 0.7;
        transition: opacity 0.2s;
      }

      button:not(:disabled):hover {
        opacity: 1;
      }

      button:disabled {
        opacity: 0.5;
        cursor: default;
      }

      /* Initial pulse animation */
      @keyframes pulse {
        0% { transform: scale(1); opacity: 0.7; }
        50% { transform: scale(1.1); opacity: 1; }
        100% { transform: scale(1); opacity: 0.7; }
      }

      button.initial {
        animation: pulse 2s infinite;
      }

      button.initial:hover {
        animation: none;
      }

      /* Loading spinner */
      @keyframes spin {
        to { transform: rotate(360deg); }
      }

      .spinner {
        display: none;
        position: absolute;
        inset: 0;
        border: 2px solid rgba(255,255,255,0.1);
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

      /* Hint text */
      .hint {
        font-size: 0.75rem;
        color: rgba(255,255,255,0.6);
        transition: opacity 0.3s;
        margin-left: 0.5rem;
      }

      .hint.hidden {
        opacity: 0;
      }

      /* SVG icons */
      .icon {
        width: 24px;
        height: 24px;
        fill: currentColor;
      }

      .play-icon {
        width: 0;
        height: 0;
        border-style: solid;
        border-width: 8px 0 8px 12px;
        border-color: transparent transparent transparent currentColor;
      }

      .pause-icon {
        width: 12px;
        height: 14px;
        border-left: 3px solid currentColor;
        border-right: 3px solid currentColor;
      }
    `;

    const template = document.createElement('template');
    template.innerHTML = `
      <div class="container">
        <div class="button-container">
          <button class="initial" aria-label="Play/Pause" data-state="stopped">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <polygon points="5 3 19 12 5 21 5 3"></polygon>
            </svg>
          </button>
          <div class="spinner"></div>
        </div>
        <div class="hint">Click play to begin</div>
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
    const hint = this.shadowRoot?.querySelector('.hint');

    if (!button) return;

    const isPlaying = button.getAttribute('data-state') === 'playing';

    if (!isPlaying) {
      // Remove initial pulse animation and hint
      button.classList.remove('initial');
      hint?.classList.add('hidden');

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
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor">
        <polygon points="5 3 19 12 5 21 5 3"></polygon>
      </svg>
    `;
  }

  private getStopIcon(): string {
    return `
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor">
        <rect x="6" y="4" width="4" height="16"></rect>
        <rect x="14" y="4" width="4" height="16"></rect>
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