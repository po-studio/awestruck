package webrtc

import (
	"fmt"
	"net/http"

	"github.com/po-studio/go-webrtc-server/session"
	sc "github.com/po-studio/go-webrtc-server/supercollider"
)

// HandleSynthCode serves the currently playing synth's code
func HandleSynthCode(w http.ResponseWriter, r *http.Request) {
	// Get session ID from header
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	// Get session
	session, err := session.GetOrCreateSession(r, w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get or create session: %v", err), http.StatusInternalServerError)
		return
	}

	// Get synth
	synthInstance, ok := session.Synth.(*sc.SuperColliderSynth)
	if !ok || synthInstance == nil {
		http.Error(w, "No active synth", http.StatusNotFound)
		return
	}

	// Get the synth code
	code, err := synthInstance.GetSynthCode()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get synth code: %v", err), http.StatusInternalServerError)
		return
	}

	// Set content type and write response
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(code))
}
