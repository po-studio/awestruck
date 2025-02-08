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
        display: block;
        width: 100%;
        height: 100%;
        min-height: 200px;
      }
      
      pre {
        margin: 0;
        padding: 1rem;
        height: 100%;
        background: #1a1a1a;
        border-radius: 0.5rem;
        overflow: auto;
        font-family: 'IBM Plex Mono', monospace;
        font-size: 0.9rem;
        line-height: 1.5;
      }
      
      code {
        display: block;
        opacity: 0;
        transform: translateY(10px);
        transition: opacity 0.3s ease, transform 0.3s ease;
      }
      
      code.visible {
        opacity: 1;
        transform: translateY(0);
      }
      
      /* Syntax highlighting */
      .token.comment { color: #666; }
      .token.keyword { color: #c678dd; }
      .token.string { color: #98c379; }
      .token.number { color: #d19a66; }
      .token.function { color: #61afef; }
      .token.operator { color: #56b6c2; }
      
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