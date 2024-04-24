![brandmark](https://github.com/po-studio/awestruck/assets/1250151/84795f2f-31de-4db3-b653-becab5dc06b9)

# Awestruck
## Real-Time Audio Synthesis, Streaming, & Manipulation

Awestruck aims to provide a framework for real-time, server-driven audio synthesis and streaming. It uses SuperCollider, a powerful language for audio programming, but any number of synthesis engines could be supported.

Along with SuperCollider, Awestruck uses:
* [JACK](https://jackaudio.org/) as a sound server API for low-latency connections
* [GStreamer](https://gstreamer.freedesktop.org/documentation/?gi-language=c) for creating audio pipelines
* [Pion/WebRTC](https://github.com/pion/webrtc) for capturing the GStreamer audio and streaming it over the web

## Why?
There are client-side audio frameworks such as [Tone.js](https://tonejs.github.io/), which are powerful in their own right. However, server-driven audio synthesis allows for using tools like SuperCollider, which offer more flexibility, complex audio processing, and algorithmic composition. It also paves the way for AI-powered audio synthesis and speech models, which are limited in client environments.

While this repo doesn't currently include methods for controlling the synth from client requests, this is possible with OSC and old-fashioned JSON payloads (though protobufs could make more sense if latency is of utmost concern).

The video demo below demonstrates a deployed instance of Awestruck which is controlled via remote requests.

## Demo
https://www.youtube.com/watch?v=iEC6-pBFj2Q

## Contents
This repository contains a Makefile and Docker configuration for building and running a demo that:

* Allows a client request a WebRTC connection to a server
* Headlessly starts SuperCollider with a random, hard-coded [.scd](https://sctweets.tumblr.com/) file.
* Pipes SuperCollider audio through the GStreamer JACK audio source
* Plays audio through the browser

## Prerequisites

* Docker
* NOTE: Docker Desktop does not easily allow for enabling IPv6, which is needed to connect to STUN. One fix is to use [OrbStack](https://orbstack.dev/), which let's you easily enable IPv6

## Getting Started

To start using this repository, clone it to your local machine and navigate into the directory:

```
make build
make up
```

To gracefully stop all processes:
```
make down
```

This should boot up the server and open a browser window. If you do not see a browser window, go to:
* localhost:8080
* Click "Start Random Synth" – view the browser console logs and you'll see the connection take place
* View the server logs to see the handshakes occur, the connection to succeed, and SuperCollider to start
* You should then hear audio

## Thoughts
The synth examples in the `supercollider` directory are taken from simple "SC tweets" under 140 characters in length. However, SuperCollider can be used to create sophisticated [music](https://open.spotify.com/track/4VecDB1uhp44posWgt85yN?si=b226049745f14d82) beyond just bloops and bleeps. I started this project years ago because this piece by Jonatan Liljedahl fascinates me.

Now, I'm interested applications/integrations with AIs. This could include:
* LLM-driven algorithmic composition. In an ideal scenario, given a prompt for a style of music, the LLM could write the .scd code before streaming it, with rapid feedback for the listener/co-composer. Unfortunately, there is little SuperCollider code out there for AIs to train on. This may change.
* Streaming of text-to-speech audio that must originate from a server environment.