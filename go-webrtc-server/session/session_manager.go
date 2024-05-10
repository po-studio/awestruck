package session

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/pion/webrtc/v3"
	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/synth"
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
	Synth             synth.Synth
	AudioSrc          *string
	SynthPort         int
}

func GetOrCreateSession(r *http.Request, w http.ResponseWriter) (*AppSession, bool) {
	sessionID, ok := getSessionIDFromHeader(r)
	if !ok {
		log.Println("No session ID provided in the header")
		http.Error(w, "No session ID provided", http.StatusBadRequest)
		return nil, false
	}

	appSession, exists := sessionManager.GetSession(sessionID)
	if !exists {
		appSession = sessionManager.CreateSession(sessionID)
	}

	return appSession, true
}

func StopAllProcesses(appSession *AppSession) {
	fmt.Println("Stopping all processes...")

	if appSession.Synth != nil {
		appSession.Synth.Stop()
	}

	if err := disconnectJackPorts(appSession); err != nil {
		fmt.Println("Error disconnecting JACK ports:", err)
	} else {
		fmt.Println("JACK ports disconnected successfully.")
	}

	if appSession.PeerConnection != nil {
		if err := appSession.PeerConnection.Close(); err != nil {
			fmt.Println("Error closing peer connection:", err)
		} else {
			fmt.Println("Peer connection closed successfully.")
		}
		appSession.PeerConnection = nil
	}

	if appSession.GStreamerPipeline != nil {
		appSession.GStreamerPipeline.Stop()
		appSession.GStreamerPipeline = nil
	}

	fmt.Println("All processes have been stopped.")
}

func disconnectJackPorts(appSession *AppSession) error {
	webrtcPorts, err := GetGStreamerJackPorts(appSession)
	if err != nil {
		return fmt.Errorf("error finding JACK ports: %w", err)
	}

	var disconnectErrors []string
	for _, webrtcPort := range webrtcPorts {
		if err := disconnectPort("SuperCollider:out_1", webrtcPort); err != nil {
			disconnectErrors = append(disconnectErrors, err.Error())
		}
	}

	if len(disconnectErrors) > 0 {
		return fmt.Errorf("failed to disconnect some ports: %s", strings.Join(disconnectErrors, "; "))
	}
	return nil
}

func GetGStreamerJackPorts(appSession *AppSession) ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var gstJackPorts []string
	prefix := "webrtc-server"

	log.Println("appSession.Id: ", appSession.Id)
	for _, port := range ports {
		if strings.HasPrefix(port, prefix) && strings.Contains(port, appSession.Id) {
			gstJackPorts = append(gstJackPorts, port)
		}
	}

	return gstJackPorts, nil
}

func disconnectPort(outputPort, inputPort string) error {
	cmd := exec.Command("jack_disconnect", outputPort, inputPort)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disconnect %s from %s: %w", outputPort, inputPort, err)
	}
	return nil
}

func getSessionIDFromHeader(r *http.Request) (string, bool) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		return "", false
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
	appSession.Synth = synth.NewSuperColliderSynth(id)

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
