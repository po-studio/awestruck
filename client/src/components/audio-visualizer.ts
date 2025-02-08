// handles real-time waveform visualization of audio data
export class AudioVisualizer extends HTMLElement {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private resizeObserver: ResizeObserver;
  private animationFrame?: number;
  private analyser?: AnalyserNode;

  constructor() {
    super();
    this.canvas = document.createElement('canvas');
    const ctx = this.canvas.getContext('2d');
    if (!ctx) throw new Error('Could not get canvas context');
    this.ctx = ctx;
    
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
    const dpr = window.devicePixelRatio || 1;
    
    this.canvas.width = rect.width * dpr;
    this.canvas.height = rect.height * dpr;
    
    this.canvas.style.width = `${rect.width}px`;
    this.canvas.style.height = `${rect.height}px`;
    
    this.ctx.scale(dpr, dpr);
  }

  private setupStyles(): void {
    const style = document.createElement('style');
    style.textContent = `
      :host {
        display: block;
        width: 100%;
        height: 80px;
        min-height: 60px;
        max-height: 120px;
      }
      
      canvas {
        width: 100%;
        height: 100%;
        background: #111;
        border-radius: 0.5rem;
      }
      
      @media (max-width: 640px) {
        :host {
          height: 60px;
        }
      }
    `;
    
    this.shadowRoot?.appendChild(style);
    this.shadowRoot?.appendChild(this.canvas);
  }

  public setAnalyser(analyser: AnalyserNode): void {
    this.analyser = analyser;
    this.startVisualization();
  }

  private startVisualization(): void {
    if (!this.analyser) return;

    const bufferLength = this.analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);

    const draw = () => {
      this.animationFrame = requestAnimationFrame(draw);

      this.analyser!.getByteTimeDomainData(dataArray);

      this.ctx.fillStyle = '#111';
      this.ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);

      this.ctx.lineWidth = 2;
      this.ctx.strokeStyle = '#FF0080';
      this.ctx.beginPath();

      const sliceWidth = this.canvas.width / bufferLength;
      let x = 0;

      for (let i = 0; i < bufferLength; i++) {
        const v = dataArray[i] / 128.0;
        const y = (v * this.canvas.height) / 2;

        if (i === 0) {
          this.ctx.moveTo(x, y);
        } else {
          this.ctx.lineTo(x, y);
        }

        x += sliceWidth;
      }

      this.ctx.lineTo(this.canvas.width, this.canvas.height / 2);
      this.ctx.stroke();
    };

    draw();
  }

  disconnectedCallback(): void {
    this.resizeObserver.disconnect();
    if (this.animationFrame) {
      cancelAnimationFrame(this.animationFrame);
    }
  }
}

customElements.define('audio-visualizer', AudioVisualizer); 