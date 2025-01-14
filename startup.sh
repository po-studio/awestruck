#!/bin/bash
set -e

# why we need explicit jack configuration:
# - ensures consistent audio buffering across environments
# - prevents xruns in cloud deployment
# - maintains low latency while being stable
jackd -d dummy -r -p ${JACK_PORT_MAX:=128} &
JACK_PID=$!

sleep 2

if ! kill -0 $JACK_PID 2>/dev/null; then
    echo "JACK failed to start"
    exit 1
fi

echo "Starting Go WebRTC server..."
exec ./webrtc-server