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
        opacity: 1 !important;
        margin: 0 !important;
        padding: 0 !important;
        line-height: 0 !important;
        background: transparent !important;
        overflow: visible !important;
      }
      
      pre {
        margin: 0 !important;
        padding: 0 1rem !important;
        min-height: 100% !important;
        background: transparent !important;
        color: #000000 !important;
        border: none !important;
        border-radius: 0 !important;
        overflow-y: auto !important;
        overflow-x: auto !important;
        font-family: 'IBM Plex Mono', monospace !important;
        font-size: 0.9rem !important;
        font-weight: 400 !important;
        line-height: 1.5 !important;
        display: block !important;
        scroll-behavior: smooth !important;
        border-top: 1px solid rgba(0,0,0,0.1) !important; /* Add subtle border */
      }
      
      code {
        display: block !important;
        background: transparent !important;
        color: #000000 !important;
        white-space: pre !important;
        margin: 0 !important;
        padding: 0 !important;
      }

      :host(.active) pre,
      :host(.active) code {
        background-color: #ffffff !important;
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
          font-size: 0.8rem !important;
          padding: 0.75rem !important;
          line-height: 1.4 !important;
        }

        .code-line {
          padding: 0.05rem 0;  // Slightly tighter spacing on mobile
        }
      }

      /* Add horizontal scrolling with touch support */
      pre {
        -webkit-overflow-scrolling: touch !important;
        scrollbar-width: thin !important;
      }

      /* Custom scrollbar styling */
      pre::-webkit-scrollbar {
        height: 4px !important;
        width: 4px !important;
      }

      pre::-webkit-scrollbar-track {
        background: #f0f0f0 !important;
      }

      pre::-webkit-scrollbar-thumb {
        background: #999 !important;
        border-radius: 2px !important;
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

    // Set the highlighted content directly without wrapping or animation
    this.code.classList.add('visible');
  }

  public setSessionManager(manager: any): void {
    this.sessionManager = manager;
  }

  // Add method to show the code viewer
  public show(): void {
    this.classList.add('active');
    const container = document.getElementById('code-container');
    const mainContainer = container?.parentElement;

    if (container && mainContainer) {
      // First make it visible with 0 height
      container.style.display = 'block';

      // Force a reflow before setting initial transition values
      container.offsetHeight; // Force reflow

      // Set initial state for transition
      container.style.height = '0';
      container.style.opacity = '0';

      // Add one-time transition listener
      const onTransitionStart = () => {
        // Start scrolling as soon as the transition begins
        const duration = 300;
        const startTime = performance.now();
        const startScroll = container.scrollTop;
        const endScroll = container.scrollHeight;

        const animateScroll = (currentTime: number) => {
          const elapsed = currentTime - startTime;
          const progress = Math.min(elapsed / duration, 1);

          const easeOut = (t: number) => 1 - Math.pow(1 - t, 3);
          const currentProgress = easeOut(progress);

          container.scrollTop = startScroll + (endScroll - startScroll) * currentProgress;
          if (this.pre) {
            this.pre.scrollTop = container.scrollTop;
          }

          if (progress < 1) {
            requestAnimationFrame(animateScroll);
          }
        };

        requestAnimationFrame(animateScroll);
      };

      container.addEventListener('transitionstart', onTransitionStart, { once: true });

      // Use double requestAnimationFrame to ensure styles are applied
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          const viewportHeight = window.innerHeight;
          const headerHeight = 200;
          const footerHeight = 60;
          const visualizerHeight = 80;
          const controlsHeight = 52;

          // Add some padding to ensure footer is comfortably visible
          const bottomPadding = 20;

          // Calculate max container height with padding
          const maxContainerHeight = viewportHeight - headerHeight - footerHeight - bottomPadding;
          const maxCodeHeight = maxContainerHeight - visualizerHeight - controlsHeight;

          // Trigger transition
          container.style.height = `${maxCodeHeight}px`;
          container.style.maxHeight = `${maxCodeHeight}px`;
          container.style.opacity = '1';
        });
      });
    }
  }

  // Add hide method alongside show
  public hide(): void {
    this.classList.remove('active');
    const container = document.getElementById('code-container');
    const mainContainer = container?.parentElement;

    if (container && mainContainer) {
      // Trigger transition to 0 height
      container.style.height = '0';
      container.style.opacity = '0';

      // Wait for transition to complete before hiding and resetting scroll
      setTimeout(() => {
        container.style.display = 'none';
        // Reset scroll position after container is hidden
        container.scrollTop = 0;
        if (this.pre) {
          this.pre.scrollTop = 0;
        }
      }, 300); // Match transition duration
    }
  }
}

customElements.define('code-viewer', CodeViewer); 