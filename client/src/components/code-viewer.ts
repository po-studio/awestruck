import 'prismjs';
import 'prismjs/components/prism-supercollider';

// handles code display with syntax highlighting
export class CodeViewer extends HTMLElement {
  private pre: HTMLPreElement;
  private code: HTMLElement;
  private sessionManager: any; // Will be set via public method
  
  constructor() {
    super();
    this.pre = document.createElement('pre');
    this.code = document.createElement('code');
    this.code.className = 'language-supercollider';
    this.pre.appendChild(this.code);
    this.setupComponent();
  }

  private setupComponent(): void {
    const shadow = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    
    style.textContent = `
      :host {
        display: block !important;
        width: 100% !important;
        height: 0 !important;  // Start with 0 height
        min-height: 0 !important;  // Start with 0 min-height
        opacity: 0;  // Start hidden
        transition: all 0.3s ease-out;  // Smooth transition
      }

      :host(.active) {
        height: 100% !important;
        min-height: 200px !important;
        opacity: 1;
      }
      
      pre {
        margin: 0 !important;
        padding: 1rem !important;
        height: 100% !important;
        background-color: #ffffff !important;  // white background
        color: #000000 !important;  // black text
        border: none !important;  // remove border
        border-radius: 0 0 0.5rem 0.5rem !important;
        overflow: auto !important;
        font-family: 'IBM Plex Mono', monospace !important;
        font-size: 0.9rem !important;
        font-weight: 400 !important;
        line-height: 1.5 !important;
      }
      
      code {
        display: block !important;
        background-color: #ffffff !important;
        color: #000000 !important;
      }

      /* Greyscale syntax highlighting with typography */
      .token {
        color: #222222 !important;  /* default color */
        font-weight: 400 !important;
      }

      .token.comment { 
        color: #888888 !important;
        font-style: italic !important;
        font-weight: 300 !important;
      }

      .token.keyword { 
        color: #111111 !important;
        font-weight: 700 !important;
      }

      .token.function { 
        color: #222222 !important;
        font-weight: 600 !important;
      }

      .token.string { 
        color: #444444 !important;
        font-style: italic !important;
      }

      .token.number { 
        color: #333333 !important;
        font-weight: 500 !important;
      }

      .token.operator { 
        color: #666666 !important;
        font-weight: 400 !important;
      }

      .token.punctuation { 
        color: #777777 !important;
        font-weight: 300 !important;
      }

      .token.parameter { 
        color: #555555 !important;
        font-style: italic !important;
      }

      /* Special SuperCollider tokens */
      .token.class-name {
        color: #222222 !important;
        font-weight: 700 !important;
      }

      .token.method {
        color: #333333 !important;
        font-weight: 600 !important;
      }

      .token.variable {
        color: #444444 !important;
        font-style: italic !important;
      }

      @media (max-width: 640px) {
        pre {
          font-size: 0.8rem;
          padding: 0.75rem;
        }
      }

      .code-line {
        display: inline-block;
        width: 100%;
        opacity: 0;
      }

      .code-line.animate {
        opacity: 1;
      }
    `;
    
    shadow.appendChild(style);
    shadow.appendChild(this.pre);
  }

  public async loadCode(): Promise<void> {
    if (!this.sessionManager) return;

    try {
      const response = await fetch('/synth-code', {
        headers: {
          'X-Session-ID': this.sessionManager.getSessionId(),
          'Accept': 'text/plain'
        }
      });

      if (!response.ok) {
        throw new Error(`Failed to load code: ${response.status}`);
      }

      const code = await response.text();
      this.setCode(code);
    } catch (error) {
      console.error('Failed to load code:', error);
      this.setCode('// Failed to load synth code');
    }
  }

  private async setCode(content: string): Promise<void> {
    // Clear previous content
    this.code.textContent = '';
    this.code.textContent = content;
    
    // Highlight with Prism
    // @ts-ignore: Prism is loaded globally
    Prism.highlightElement(this.code);
    
    // Get the highlighted HTML content and wrap each line
    const wrappedContent = this.code.innerHTML
      .split('\n')
      .filter(line => line.trim())  // Remove empty lines
      .map(line => `<span class="code-line">${line}</span>`)
      .join('');  // Remove the \n in join()
    
    // Update the content with our wrapped version
    this.code.innerHTML = wrappedContent;

    // Add streaming animation styles
    const style = document.createElement('style');
    style.textContent = `
      @keyframes streamText {
        from { 
          clip-path: inset(0 100% 0 0);
          opacity: 0;
        }
        to { 
          clip-path: inset(0 0 0 0);
          opacity: 1;
        }
      }

      .code-line {
        display: block;
        width: 100%;
        animation: streamText 0.5s linear forwards;
        white-space: pre;
        position: relative;
        line-height: 1.2;  // Tighter line height
        margin: 0;         // Remove margins
        padding: 0;        // Remove padding
      }

      /* Greyscale syntax highlighting */
      .token.comment { color: #888888 !important; }
      .token.keyword { color: #222222 !important; }
      .token.string { color: #444444 !important; }
      .token.number { color: #333333 !important; }
      .token.function { color: #111111 !important; }
      .token.operator { color: #555555 !important; }
      .token.punctuation { color: #666666 !important; }
      .token.parameter { color: #777777 !important; }
    `;
    
    // Remove any previous animation styles
    this.shadowRoot?.querySelectorAll('style').forEach(s => {
      if (s.textContent?.includes('@keyframes streamText')) {
        s.remove();
      }
    });
    
    this.shadowRoot?.appendChild(style);

    // Wait for syntax highlighting to complete
    await new Promise(resolve => setTimeout(resolve, 0));

    // Animate each line
    const codeLines = this.code.querySelectorAll('.code-line');
    const duration = 500; // Animation duration in ms
    
    codeLines.forEach((line, i) => {
      (line as HTMLElement).style.animationDelay = `${50}ms`;
      (line as HTMLElement).style.animationDuration = `${duration}ms`;
    });

    // Add visible class after animation
    setTimeout(() => {
      this.code.classList.add('visible');
    }, duration + 100);
  }

  public setSessionManager(manager: any): void {
    this.sessionManager = manager;
  }

  // Add method to show the code viewer
  public show(): void {
    this.classList.add('active');
  }

  // Add hide method alongside show
  public hide(): void {
    this.classList.remove('active');
  }
}

customElements.define('code-viewer', CodeViewer); 