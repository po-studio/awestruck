#!/bin/bash
set -e

# why we need explicit jack configuration:
# - ensures consistent audio buffering across environments
# - prevents xruns in cloud deployment
# - maintains low latency while being stable
echo "Starting JACK with buffer_size=${JACK_BUFFER_SIZE} rate=${JACK_SAMPLE_RATE}..."
jackd -d dummy \
  -r \
  -p ${JACK_BUFFER_SIZE:=2048} \
  -r ${JACK_SAMPLE_RATE:=48000} \
  -C 2 -P 2 \
  -m &  # Enable monitor ports
JACK_PID=$!

sleep 2

if ! kill -0 $JACK_PID 2>/dev/null; then
    echo "JACK failed to start"
    exit 1
fi

# why we need to ensure ports are connected:
# - dummy driver needs explicit connections
# - ensures audio flow path is established
# - prevents webrtc connection failures
echo "Connecting JACK ports..."
jack_connect system:capture_1 system:playback_1 || true
jack_connect system:capture_2 system:playback_2 || true

echo "Starting Go WebRTC server..."
exec ./webrtc-server