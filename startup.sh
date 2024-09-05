#!/bin/bash
set -e

echo "Starting startup script..."

# Print system information
uname -a
cat /etc/os-release

# Check if jackd is installed
which jackd || echo "jackd not found"

# Print JACK version
jackd --version

# Start jackd with dummy backend
echo "Starting JACK..."
jackd -R -d dummy -r $JACK_SAMPLE_RATE &
JACK_PID=$!

# Wait for JACK to start
sleep 5

# Check if JACK is running
if ! kill -0 $JACK_PID 2>/dev/null; then
    echo "JACK failed to start"
    exit 1
fi

# Start the Go WebRTC server
echo "Starting Go WebRTC server..."
./webrtc-server -host 0.0.0.0