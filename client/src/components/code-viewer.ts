import 'prismjs';
import 'prismjs/components/prism-supercollider';

// handles code display with syntax highlighting
export class CodeViewer extends HTMLElement {
  private pre: HTMLPreElement;
  private code: HTMLElement;
  private sessionManager: any; // Will be set via public method
  private observer: MutationObserver;

  constructor() {
    super();
    this.pre = document.createElement('pre');
    this.code = document.createElement('code');
    this.code.className = 'language-supercollider';
    this.pre.appendChild(this.code);
    this.setupComponent();
    this.observeDarkMode();
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
        color: inherit !important;
      }
      
      code {
        font-family: 'IBM Plex Mono', monospace !important;
        font-size: 14px !important;
        line-height: 1.5 !important;
        padding: 0 !important;
        background: transparent !important;
        color: inherit !important;
      }
      
      /* Dark mode styles within shadow DOM */
      :host-context(.dark) pre,
      :host-context(.dark) code {
        background-color: #080808 !important;
        color: #e1e1e1 !important;
      }
      
      :host-context(.dark) .token.comment {
        color: #6a9955 !important;
      }
      
      :host-context(.dark) .token.keyword {
        color: #c586c0 !important;
      }
      
      :host-context(.dark) .token.string {
        color: #ce9178 !important;
      }
      
      :host-context(.dark) .token.number {
        color: #b5cea8 !important;
      }
      
      :host-context(.dark) .token.function {
        color: #dcdcaa !important;
      }
      
      /* Check CSS custom property as fallback */
      @media (prefers-color-scheme: dark) {
        pre, code {
          background-color: #080808 !important;
          color: #e1e1e1 !important;
        }
      }
    `;

    shadow.appendChild(style);
    shadow.appendChild(this.pre);
  }

  private observeDarkMode(): void {
    // Apply dark mode initially
    this.updateTheme();

    // Watch for changes to the dark mode class on html element
    this.observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        if (mutation.attributeName === 'class') {
          this.updateTheme();
        }
      });
    });

    this.observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class']
    });
  }

  private updateTheme(): void {
    const isDarkMode = document.documentElement.classList.contains('dark');
    if (isDarkMode) {
      this.setAttribute('theme', 'dark');
    } else {
      this.removeAttribute('theme');
    }
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

  disconnectedCallback() {
    if (this.observer) {
      this.observer.disconnect();
    }
  }
}

customElements.define('code-viewer', CodeViewer); 