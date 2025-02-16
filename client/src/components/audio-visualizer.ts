// handles real-time frequency bar visualization of audio data
export class AudioVisualizer extends HTMLElement {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private resizeObserver: ResizeObserver;
  private animationFrame?: number;
  private analyser?: AnalyserNode;
  private dpr: number;

  constructor() {
    super();
    this.canvas = document.createElement('canvas');
    const ctx = this.canvas.getContext('2d');
    if (!ctx) throw new Error('Could not get canvas context');
    this.ctx = ctx;
    this.dpr = window.devicePixelRatio || 1;
    
    this.setupCanvas();
    this.attachShadow({ mode: 'open' });
    this.setupStyles();
  }

  private setupCanvas(): void {
    this.resizeObserver = new ResizeObserver(() => {
      this.updateCanvasSize();
    });
    this.resizeObserver.observe(this);
    this.updateCanvasSize();
  }

  private updateCanvasSize(): void {
    const rect = this.getBoundingClientRect();
    
    // Set canvas size accounting for device pixel ratio
    this.canvas.width = rect.width * this.dpr;
    this.canvas.height = rect.height * this.dpr;
    
    // Set display size
    this.canvas.style.width = `${rect.width}px`;
    this.canvas.style.height = `${rect.height}px`;
    
    // Scale context for retina display
    this.ctx.scale(this.dpr, this.dpr);
  }

  private setupStyles(): void {
    const style = document.createElement('style');
    style.textContent = `
      :host {
        display: block;
        width: 100%;
        height: 80px;  // reduced from 120px
        min-height: 60px;  // reduced from 80px
        background-color: #1a1a1a;
        overflow: hidden;
      }
      
      canvas {
        width: 100%;
        height: 100%;
        background-color: #1a1a1a;
        border: none;  // remove border
        border-radius: 0.5rem 0.5rem 0 0;  // round top corners only
      }
      
      @media (max-width: 640px) {
        :host {
          height: 80px;
        }
      }
    `;
    
    this.shadowRoot?.appendChild(style);
    this.shadowRoot?.appendChild(this.canvas);
  }

  private draw(): void {
    if (!this.analyser) return;
    this.animationFrame = requestAnimationFrame(this.draw.bind(this));
    
    const width = this.canvas.width / this.dpr;
    const height = this.canvas.height / this.dpr;
    const bufferLength = this.analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);

    this.ctx.save();
    this.ctx.scale(this.dpr, this.dpr);
    
    // Clear background
    this.ctx.fillStyle = '#1a1a1a';
    this.ctx.fillRect(0, 0, width, height);
    
    // Draw frequency bars
    this.analyser.getByteFrequencyData(dataArray);
    const barWidth = width / bufferLength * 2.5;
    let x = 0;
    
    // Create gradient for bars
    for (let i = 0; i < bufferLength; i++) {
        const barHeight = (dataArray[i] / 255) * height;
        
        // Create vertical gradient for each bar
        const gradient = this.ctx.createLinearGradient(0, height - barHeight, 0, height);
        gradient.addColorStop(0, '#ffffff');   // white at top
        gradient.addColorStop(1, '#cccccc');   // light grey at bottom
        
        this.ctx.fillStyle = gradient;
        this.ctx.fillRect(x, height - barHeight, barWidth, barHeight);
        x += barWidth + 1;
    }

    this.ctx.restore();
  }

  public setAnalyser(analyser: AnalyserNode): void {
    this.analyser = analyser;
    this.draw();
  }

  disconnectedCallback(): void {
    this.resizeObserver.disconnect();
    if (this.animationFrame) {
      cancelAnimationFrame(this.animationFrame);
    }
  }
}

customElements.define('audio-visualizer', AudioVisualizer); 