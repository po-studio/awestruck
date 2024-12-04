package session

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/po-studio/go-webrtc-server/synth"
)

var sessionManager = SessionManager{
	Sessions: make(map[string]*AppSession),
}

type SessionManager struct {
	Sessions map[string]*AppSession
	mutex    sync.Mutex
}

func GetOrCreateSession(r *http.Request, w http.ResponseWriter) (*AppSession, error) {
	sessionID, ok := getSessionIDFromHeader(r)
	if !ok {
		log.Println("No session ID provided in the header")
		return nil, fmt.Errorf("no session ID provided")
	}

	appSession, exists := sessionManager.GetSession(sessionID)
	if !exists {
		appSession = sessionManager.CreateSession(sessionID)
	}

	return appSession, nil
}

func getSessionIDFromHeader(r *http.Request) (string, bool) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		return "", false
	}
	return sessionID, true
}

func (sm *SessionManager) CreateSession(id string) *AppSession {
	log.Printf("[SESSION] Creating new session: %s", id)
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	appSession := &AppSession{}
	appSession.Id = id

	audioSrcFlag := fmt.Sprintf("audio-src-%s", id)
	// audioSrcConfig := fmt.Sprintf(`jackaudiosrc client-name=%s buffer-time=40000 !
	//                audio/x-raw ! level name=audiolevel interval=100000000 post-messages=true !
	//                audioconvert ! audioresample !
	//                opusenc complexity=10 ! appsink name=sink sync=false`, id)
	audioSrcConfig := fmt.Sprintf("jackaudiosrc name=%s ! audioconvert ! audioresample", id)

	appSession.AudioSrc = flag.String(audioSrcFlag, audioSrcConfig, "GStreamer audio src")
	appSession.Synth = synth.NewSuperColliderSynth(id)

	log.Printf("[AUDIO] Configuring audio pipeline for session %s: %s", id, audioSrcConfig)

	sm.Sessions[id] = appSession

	log.Printf("[SESSION] Session %s created successfully with audio source and synth", id)
	return appSession
}

func (sm *SessionManager) GetSession(id string) (*AppSession, bool) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	session, exists := sm.Sessions[id]
	return session, exists
}

// TODO either delete this or utilize it within app_session's StopAllProcesses()
func (sm *SessionManager) DeleteSession(id string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	delete(sm.Sessions, id)
}
