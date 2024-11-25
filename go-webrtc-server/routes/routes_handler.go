package routes

import (
	"net/http"

	"github.com/gorilla/mux"

	webrtc "github.com/po-studio/go-webrtc-server/webrtc"
)

func NewRouter() *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/offer", webrtc.HandleOffer).Methods("POST")
	router.HandleFunc("/stop", webrtc.HandleStop).Methods("POST")
	router.HandleFunc("/", serveHome).Methods("GET")
	router.HandleFunc("/turn-credentials", webrtc.HandleTURNCredentials).Methods("GET")
	router.HandleFunc("/ice-candidate", webrtc.HandleICECandidate).Methods("POST")
	router.PathPrefix("/").Handler(serveStatic("./client/"))
	return router
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./client/index.html")
}

func serveStatic(path string) http.Handler {
	return http.FileServer(http.Dir(path))
}
