#!/bin/bash
set -e

echo "Starting JACK..."
jackd -r -d dummy -r $JACK_SAMPLE_RATE &
JACK_PID=$!

sleep 2

if ! kill -0 $JACK_PID 2>/dev/null; then
    echo "JACK failed to start"
    exit 1
fi

echo "Starting Go WebRTC server..."
exec ./webrtc-server