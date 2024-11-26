package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	sessionManager "github.com/po-studio/go-webrtc-server/session"
	"github.com/po-studio/go-webrtc-server/types"
)

// just dummy "password" for now
func generateTURNCredentials(secret string) types.TURNCredentials {
	username := fmt.Sprintf("awestruck-%d", time.Now().Unix())
	log.Printf("[TURN] Generating credentials with username: %s (password length: %d)",
		username, len(secret))

	return types.TURNCredentials{
		Username: username,
		Password: secret,
		URLs: []string{
			"turn:turn.awestruck.io:3478",
			"turns:turn.awestruck.io:5349",
		},
	}
}

func HandleTURNCredentials(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	log.Printf("[TURN] Handling credentials request for session: %s from IP: %s",
		sessionID, r.RemoteAddr)

	// Get or create session
	appSession, err := sessionManager.GetOrCreateSession(r, w)
	if err != nil {
		log.Printf("[TURN][ERROR] Failed to get/create session %s: %v", sessionID, err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}
	log.Printf("[TURN] Successfully got/created session: %s", sessionID)

	// Get TURN secret
	secret, err := getTURNSecretFromSSM()
	if err != nil {
		log.Printf("[TURN][ERROR] Failed to get TURN secret for session %s: %v", sessionID, err)
		http.Error(w, "Configuration error", http.StatusInternalServerError)
		return
	}
	log.Printf("[TURN] Successfully retrieved TURN secret for session: %s", sessionID)

	// Generate credentials
	credentials := generateTURNCredentials(secret)
	log.Printf("[TURN] Generated credentials for session %s: username=%s, urls=%v",
		sessionID, credentials.Username, credentials.URLs)

	// Store in session
	appSession.TURNCredentials = &credentials
	log.Printf("[TURN] Stored credentials in session: %s", sessionID)

	// Return credentials
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(credentials); err != nil {
		log.Printf("[TURN][ERROR] Failed to encode credentials for session %s: %v",
			sessionID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[TURN] Successfully sent credentials to client: %s", sessionID)
}

func getTURNSecretFromSSM() (string, error) {
	log.Println("Using hardcoded TURN password")
	return "password", nil
}

// func getTURNSecretFromSSM() (string, error) {
// 	log.Println("Fetching TURN secret...")

// 	if turnPassword := os.Getenv("TURN_PASSWORD"); turnPassword != "" {
// 		log.Println("Using TURN password from environment")
// 		return turnPassword, nil
// 	}

// 	log.Println("Fetching TURN password from AWS SSM...")
// 	// Fallback to AWS SSM
// 	sess, err := session.NewSession(&aws.Config{
// 		Region: aws.String("us-east-1"),
// 	})
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create AWS session: %v", err)
// 	}

// 	svc := ssm.New(sess)

// 	input := &ssm.GetParameterInput{
// 		Name:           aws.String("/awestruck/turn_password"),
// 		WithDecryption: aws.Bool(true),
// 	}

// 	result, err := svc.GetParameter(input)
// 	if err != nil {
// 		return "", err
// 	}

// 	return *result.Parameter.Value, nil
// }
