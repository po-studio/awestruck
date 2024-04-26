![brandmark](https://github.com/po-studio/awestruck/assets/1250151/84795f2f-31de-4db3-b653-becab5dc06b9)

# Awestruck

### WARNING
Protect your ears! Before streaming audio, please turn the volume on your machine DOWN, especially if you're using headphones. While I've tried to ensure the examples play at a reasonable volume, this software gets close to audio hardware and rare glitches such as amplitude spikes can occur.

## Real-Time Audio Synthesis, Streaming, & Manipulation
Awestruck aims to provide a framework for real-time, server-driven audio synthesis, streaming, and control over the Internet. It uses SuperCollider, a powerful language for audio programming, but other synthesis engines could be supported.

Awestruck uses:
* [SuperCollider](https://supercollider.github.io/) for real-time sound synthesis
* [JACK](https://jackaudio.org/) as a sound server API for low-latency connections
* [GStreamer](https://gstreamer.freedesktop.org/documentation/?gi-language=c) for creating audio pipelines
* [Pion/WebRTC](https://github.com/pion/webrtc) for streaming captured audio over the web

## Why?
There are client-side audio frameworks such as [Tone.js](https://tonejs.github.io/), which are powerful in their own right. However, server-driven audio synthesis allows for using tools like SuperCollider, which offer more flexibility, complex audio processing, and algorithmic composition. It also paves the way for AI-powered audio synthesis and speech models, which are limited in client environments.

While this repo doesn't currently include methods for controlling the synth from client requests, this is possible with [OpenSoundControl](https://ccrma.stanford.edu/groups/osc/index.html) and old-fashioned JSON payloads. Protobufs could offer more efficiency in size/speed for high-frequency, low-latency needs.

The video demo below demonstrates a deployed instance of Awestruck that is controlled via remote requests.

## Demo
https://www.youtube.com/watch?v=iEC6-pBFj2Q

## Contents
This repository contains a Makefile and Docker configuration for building and running a demo that provides a browser interface for starting the synth. Starting the synth does the following:

* Forms a Pion/WebRTC connection to the running server via handshakes
* Creates a GStreamer pipeline with JACK audio
* Headlessly starts SuperCollider with a random, hard-coded [.scd](https://sctweets.tumblr.com/) file
* Pipes SuperCollider output audio through GStreamer via JACK
* Uses Pion/WebRTC to stream the audio to the browser

## Prerequisites

* Docker
* NOTE: Docker Desktop does not easily allow for enabling IPv6, which is needed for STUN connections. One fix is to use [OrbStack](https://orbstack.dev/), which lets you easily enable IPv6.

## Getting Started

### Build & Run
To start using this repository, clone it to your local machine and navigate into the directory:

```
make build
make up
```

This should boot up the server and open a browser window. If you do not see a browser window, go to:
* localhost:8080
* Click "Start Random Synth" – view the browser console logs and you'll see the connection take place
* View the server logs to see the handshakes occur, the connection to succeed, and SuperCollider to start
* You should then hear audio

### Graceful Shutdown
To gracefully stop all processes:
```
make down
```

## Thoughts
The synth examples in the `supercollider` directory are taken from simple [SC tweets](https://sctweets.tumblr.com) under 140 characters in length, and are highly minimal. That's not to say these nano-compositions aren't interesting, just that they were written by various artists within tight constraints. However, SuperCollider can be used to create larger-scale music.

For example, Jonatan Liljedahl wrote this [track](https://open.spotify.com/track/4VecDB1uhp44posWgt85yN?si=b226049745f14d82) in ~100 lines of SC code. Imagine complex "applications" which represent pieces of music on the scale of symphonies or concertos. This is possible with SuperCollider.

I'm currently interested applications/integrations with AIs. This could include:
* Streaming of LLM-powered [text-to-speech audio](https://github.com/suno-ai/bark) that calls for server-side origins.
* LLM-driven algorithmic composition. In an ideal scenario, given a prompt for a style of music, the LLM could write the .scd code before streaming it, with rapid feedback for the listener/co-composer. Unfortunately, there is little SuperCollider code out there for AIs to train on, and even if there were, a higher-level system like [Devin](https://www.cognition-labs.com/introducing-devin) with knowledge of musical structure and aesthetics would be necessary in order to produce anything worth listening to. This may change.
