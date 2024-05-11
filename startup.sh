#!/bin/bash

# Start jackd
# NOTE the port-max should be 4x the number of simultaneous synths we can support
jackd -r --port-max 40 -d dummy -C /dev/null -P /dev/null &

echo "Current working directory: $(pwd)"

# Start the Go WebRTC server
echo "Starting Go WebRTC server..."
./webrtc-server