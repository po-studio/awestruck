package main

import (
  "os/exec"
  "encoding/json"
  "io"
  "log"
  "net/http"
  "fmt"
  "os"

  jackgst "github.com/po-studio/example-webrtc-applications/v3/gstreamer-send/jack"
  "github.com/gorilla/mux"
)

func main() {

  jackCmd := exec.Command("sh", "-c", "jackd -r -d dummy -r 44100")
  jackCmd.Stdout = os.Stdout
  err := jackCmd.Start()
  if err != nil {
    log.Fatal(err)
  }
  log.Printf("Just ran subprocess %d, exiting\n", jackCmd.Process.Pid)

  r := mux.NewRouter()

  r.HandleFunc("/token", tokenHandler).Methods("POST")
  r.PathPrefix("/").Handler(http.FileServer(http.Dir("/webserver/static")))

  port := os.Getenv("PORT")
  if port == "" {
    port = "8080"
  }

  log.Println("** Service Started on Port " + port + " **")
  http.ListenAndServe(":8080", r)
}

// var remoteTokenCh = make(chan string)

type token_struct struct {
  Token string
}

func tokenHandler(w http.ResponseWriter, r *http.Request) {
  decoder := json.NewDecoder(r.Body)
  var t token_struct
  err := decoder.Decode(&t)
  if err != nil {
    panic(err)
  }

  remoteTokenCh := make(chan string)
  jackgst.StartGStreamer(t.Token, remoteTokenCh)

  fmt.Printf("browser token main")
  fmt.Println(t.Token)

  remoteToken := <-remoteTokenCh
  fmt.Sprintf(remoteToken)

  // cmd := exec.Command("/usr/src/sccmd.sh")
  // cmd.Stdout = os.Stdout
  // err = cmd.Start()
  // if err != nil {
  //   log.Fatal(err)
  // }
  // log.Printf("Just ran subprocess %d, exiting\n", cmd.Process.Pid)

  resp := fmt.Sprintf(`{"remote_token":"%s"}`, remoteToken)
  io.WriteString(w, resp)
}
