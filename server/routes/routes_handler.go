package routes

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	synth "github.com/po-studio/server/synth"
	webrtc "github.com/po-studio/server/webrtc"
)

func NewRouter() *mux.Router {
	router := mux.NewRouter()
	// not sure if we need this...
	router.HandleFunc("/", serveHome).Methods("GET")

	// gets webrtc config including ice credentials, host, etc.
	router.HandleFunc("/config", webrtc.HandleConfig).Methods("GET")

	// for creating the webrtc offer once the client has fetched the config
	router.HandleFunc("/offer", webrtc.HandleOffer).Methods("POST")

	// stops the webrtc connection and executes synthesis/session cleanup
	router.HandleFunc("/stop", webrtc.HandleStop).Methods("POST")

	router.HandleFunc("/ice-candidate", webrtc.HandleICECandidate).Methods("POST")

	// serve static files, i.e. website
	// this is a temporary hack until we have a proper frontend
	router.PathPrefix("/").Handler(serveStatic("./client/"))

	// just for frontend -- displays the source code of the synth
	// being synthesized/streamed in real-time
	router.HandleFunc("/synth-code", webrtc.HandleSynthCode).Methods("GET")

	// experimental, for testing generative LLM synths
	// this should become a recurring background job
	router.HandleFunc("/generate-synth", synth.GenerateSynth).Methods("POST")

	return router
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	// http.ServeFile(w, r, "./client/index.html")
	http.ServeFile(w, r, "./client/index2.html")
}

func serveStatic(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set correct MIME types for different file extensions
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}

		// Use the default file server to serve the file
		http.FileServer(http.Dir(path)).ServeHTTP(w, r)
	})
}
