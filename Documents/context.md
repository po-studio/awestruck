Awestruck is a web application for real-time synthesis and streaming of audio/music over the internet. It is evolving to support live audio programming with AI co-pilots.

It includes:
* Golang webserver
* SuperCollider for audio synthesis
* JACK for audio transport
* GStreamer for audio streaming
* WebRTC/Pion for audio streaming
* Simple web interface for starting/stopping a synthesis session
* Multi-session support (simultaneous, independent audio streams)
* Stun/Turn server for NAT traversal

The goal is to allow for infinitely scalable, low-latency, high-quality audio streaming and synthesis over the internet.

Current issues:
- While WebRTC connections are being established in deployed environments, audio is not being streamed (or at least not audible to the user from the browser).
- Need to setup local development environment replicating all aspects of the production environment, including Stun/Turn server, port forwarding, etc.
- Need to set up proper CI/CD pipeline to deploy to production environment, though we are using cdktf/terraform currently, which is a good start.
- Linting/Editorconfig/Prettier/etc.
- Need to setup monitoring/alerting for the production environment
- Need to setup proper logging for the production environment
- Need to setup proper error handling/recovery/restart mechanisms for the production environment
