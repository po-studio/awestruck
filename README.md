# Awestruck – Real-Time Audio Synthesis & Streaming
##  SuperCollider, Go, Pion/WebRTC, Jack, & GStreamer

This repository contains a Makefile and Docker configuration for building and running a demo that:
* Let's a client request a WebRTC connection to a server
* Boots up SuperCollider to run a hard-coded file "liljedahl.scd" headlessly
* Pipes SuperCollider audio through the GStreamer JACK audio source

## Prerequisites

* Docker
* NOTE: Docker Desktop does not easily allow for enabling IPv6, which is needed to connect to STUN. One fix is to use OrbStack, which let's you easily enable IPv6

## Getting Started

To start using this repository, clone it to your local machine and navigate into the directory:

```
make build
make run
```

This should boot up the server, but nothing will happen until you go to:
* localhost:8080
* Click "Start Streaming" – view the browser console logs and you'll see the connection take place
* View the server logs to see the handshakes occur, the connection to succeed, and SuperCollider to start
* You should then hear audio
