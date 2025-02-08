// handles play/stop button and volume controls
export class PlaybackControls extends HTMLElement {
  private button: HTMLButtonElement;
  private audioManager?: any; // Will be set via public method
  
  constructor() {
    super();
    this.button = document.createElement('button');
    this.setupComponent();
  }

  private setupComponent(): void {
    const shadow = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    
    style.textContent = `
      :host {
        display: block;
      }
      
      button {
        width: 4rem;
        height: 4rem;
        border-radius: 50%;
        border: 2px solid #333;
        background: #222;
        color: #fff;
        cursor: pointer;
        transition: all 0.2s ease;
        display: flex;
        align-items: center;
        justify-content: center;
      }
      
      button:hover {
        background: #333;
        transform: scale(1.05);
      }
      
      button:active {
        transform: scale(0.95);
      }
      
      button[data-state="playing"] {
        background: #111;
        border-color: #FF0080;
      }
      
      @media (max-width: 640px) {
        button {
          width: 3rem;
          height: 3rem;
        }
      }
    `;
    
    this.button.setAttribute('data-state', 'stopped');
    this.button.innerHTML = this.getPlayIcon();
    
    this.button.addEventListener('click', () => {
      this.togglePlayback();
    });
    
    shadow.appendChild(style);
    shadow.appendChild(this.button);
  }

  private async togglePlayback(): Promise<void> {
    if (!this.audioManager) return;

    const isPlaying = this.button.getAttribute('data-state') === 'playing';
    
    try {
      if (isPlaying) {
        await this.audioManager.disconnect();
        this.button.setAttribute('data-state', 'stopped');
        this.button.innerHTML = this.getPlayIcon();
      } else {
        await this.audioManager.connect();
        this.button.setAttribute('data-state', 'playing');
        this.button.innerHTML = this.getStopIcon();
      }
    } catch (error) {
      console.error('Playback toggle failed:', error);
      this.button.setAttribute('data-state', 'stopped');
      this.button.innerHTML = this.getPlayIcon();
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
}

customElements.define('playback-controls', PlaybackControls); 