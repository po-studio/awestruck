#!/bin/bash
set -e

# why we need explicit jack configuration:
# - ensures consistent audio buffering across environments
# - prevents xruns in cloud deployment
# - maintains stability in non-realtime environment
jackd --no-realtime -d dummy --rate 48000 --period 1024 --wait 21333 --playback 2 --capture 2 &
JACK_PID=$!

sleep 2

if ! kill -0 $JACK_PID 2>/dev/null; then
    echo "JACK failed to start"
    exit 1
fi

echo "Starting Go WebRTC server..."
exec ./webrtc-server