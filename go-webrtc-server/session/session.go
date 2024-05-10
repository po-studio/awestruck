package session

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync"

	"github.com/pion/webrtc/v3"
	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
)

var sessionManager = SessionManager{
	Sessions: make(map[string]*AppSession),
}

type SessionManager struct {
	Sessions map[string]*AppSession
	mutex    sync.Mutex
}

type AppSession struct {
	Id                string
	PeerConnection    *webrtc.PeerConnection
	GStreamerPipeline *gst.Pipeline
	SuperColliderCmd  *exec.Cmd
	SuperColliderPort int
	AudioSrc          *string
}

func GetOrCreateSession(r *http.Request, w http.ResponseWriter) (*AppSession, bool) {
	// Extract session ID from the HTTP header
	sessionID, ok := getSessionIDFromHeader(r)
	if !ok {
		log.Println("No session ID provided in the header")
		http.Error(w, "No session ID provided", http.StatusBadRequest)
		return nil, false
	}

	// Retrieve or create a session based on the session ID
	appSession, exists := sessionManager.GetSession(sessionID)
	if !exists {
		// Optionally create a new session if one does not exist
		appSession = sessionManager.CreateSession(sessionID)
	}

	return appSession, true
}

func getSessionIDFromHeader(r *http.Request) (string, bool) {
	sessionID := r.Header.Get("X-Session-ID") // Custom header, typically prefixed with 'X-'
	if sessionID == "" {
		return "", false // No session ID found in headers
	}
	return sessionID, true
}

func (sm *SessionManager) CreateSession(id string) *AppSession {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	appSession := &AppSession{}
	appSession.Id = id

	audioSrcFlag := fmt.Sprintf("audio-src-%s", id)
	audioSrcConfig := fmt.Sprintf("jackaudiosrc name=%s ! audioconvert ! audioresample", id)

	appSession.AudioSrc = flag.String(audioSrcFlag, audioSrcConfig, "GStreamer audio src")

	sm.Sessions[id] = appSession

	return appSession
}

func (sm *SessionManager) GetSession(id string) (*AppSession, bool) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	session, exists := sm.Sessions[id]
	return session, exists
}

func (sm *SessionManager) DeleteSession(id string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	delete(sm.Sessions, id)
}
