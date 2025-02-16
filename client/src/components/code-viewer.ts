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
        height: 100% !important;
        min-height: 200px !important;
        background-color: #ffffff !important;  // white background
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
        font-family: monospace !important;
        font-size: 0.9rem !important;
        line-height: 1.5 !important;
      }
      
      code {
        display: block !important;
        background-color: #ffffff !important;
        color: #000000 !important;
      }

      /* Syntax highlighting in dark greys */
      .token.comment { color: #666666 !important; }
      .token.keyword { color: #000000 !important; }
      .token.string { color: #333333 !important; }
      .token.number { color: #222222 !important; }
      .token.function { color: #111111 !important; }
      .token.operator { color: #444444 !important; }
      
      @media (max-width: 640px) {
        pre {
          font-size: 0.8rem;
          padding: 0.75rem;
        }
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

  private setCode(content: string): void {
    this.code.textContent = content;
    // @ts-ignore: Prism is loaded globally
    Prism.highlightElement(this.code);
    requestAnimationFrame(() => {
      this.code.classList.add('visible');
    });
  }

  public setSessionManager(manager: any): void {
    this.sessionManager = manager;
  }
}

customElements.define('code-viewer', CodeViewer); 