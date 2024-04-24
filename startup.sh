#!/bin/bash

# Start jackd
jackd -r --port-max 20 -d dummy &

# Start the Go WebRTC server
echo "Starting Go WebRTC server..."
./webrtc-server