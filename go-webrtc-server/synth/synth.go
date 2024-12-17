package synth

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/po-studio/go-webrtc-server/llm"
	sc "github.com/po-studio/go-webrtc-server/supercollider"
)

type Synth interface {
	Start() error
	Stop() error
	GetPort() int
	SendPlayMessage()
	SetOnClientName(func(string))
}

type SynthType string

const (
	SuperCollider SynthType = "supercollider"
	MaxMSP        SynthType = "maxmsp"
)

const (
	DEFAULT_SYNTH_TYPE = "supercollider"
)

// this would be more aptly named "NewSuperColliderSynthSession"
// bc we're introducing logic for generating entirely new synths
// via LLMs
func NewSuperColliderSynth(id string) *sc.SuperColliderSynth {
	return &sc.SuperColliderSynth{Id: id}
}

type GenerateSynthRequest struct {
	Prompt   string `json:"prompt"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func GenerateSynth(w http.ResponseWriter, r *http.Request) {
	var req GenerateSynthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	code, err := llm.GenerateSynthCode(req.Provider, req.Prompt, req.Model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println(code)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(code))
}
