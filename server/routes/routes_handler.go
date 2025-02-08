package routes

import (
	"github.com/gorilla/mux"

	synth "github.com/po-studio/server/synth"
	webrtc "github.com/po-studio/server/webrtc"
)

func NewRouter() *mux.Router {
	router := mux.NewRouter()

	// gets webrtc config including ice credentials, host, etc.
	router.HandleFunc("/config", webrtc.HandleConfig).Methods("GET")

	// for creating the webrtc offer once the client has fetched the config
	router.HandleFunc("/offer", webrtc.HandleOffer).Methods("POST")

	// stops the webrtc connection and executes synthesis/session cleanup
	router.HandleFunc("/stop", webrtc.HandleStop).Methods("POST")

	router.HandleFunc("/ice-candidate", webrtc.HandleICECandidate).Methods("POST")

	// just for frontend -- displays the source code of the synth
	// being synthesized/streamed in real-time
	router.HandleFunc("/synth-code", webrtc.HandleSynthCode).Methods("GET")

	// experimental, for testing generative LLM synths
	// this should become a recurring background job
	router.HandleFunc("/generate-synth", synth.GenerateSynth).Methods("POST")

	return router
}
