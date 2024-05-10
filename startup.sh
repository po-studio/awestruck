#!/bin/bash

# Start jackd
# jackd -r --port-max 40 -d dummy &

jackd -r --port-max 40 -d dummy -C /dev/null -P /dev/null &

echo "Current working directory: $(pwd)"

# Start the Go WebRTC server
echo "Starting Go WebRTC server..."
./webrtc-server