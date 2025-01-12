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
	router.HandleFunc("/offer", webrtc.HandleOffer).Methods("POST")
	router.HandleFunc("/stop", webrtc.HandleStop).Methods("POST")
	router.HandleFunc("/", serveHome).Methods("GET")
	router.HandleFunc("/ice-candidate", webrtc.HandleICECandidate).Methods("POST")
	router.HandleFunc("/generate-synth", synth.GenerateSynth).Methods("POST")
	router.HandleFunc("/synth-code", webrtc.HandleSynthCode).Methods("GET")
	router.PathPrefix("/").Handler(serveStatic("./client/"))
	return router
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./client/index.html")
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
