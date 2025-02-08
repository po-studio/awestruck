// handles session management and persistence
export class SessionManager {
  private static instance: SessionManager;
  private sessionId: string;

  private constructor() {
    this.sessionId = this.loadOrCreateSessionId();
  }

  public static getInstance(): SessionManager {
    if (!SessionManager.instance) {
      SessionManager.instance = new SessionManager();
    }
    return SessionManager.instance;
  }

  public getSessionId(): string {
    return this.sessionId;
  }

  private loadOrCreateSessionId(): string {
    const stored = localStorage.getItem('sessionId');
    if (stored) return stored;

    const newId = Math.random().toString(36).substring(2, 15);
    localStorage.setItem('sessionId', newId);
    return newId;
  }
} 