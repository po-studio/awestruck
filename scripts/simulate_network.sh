#!/bin/bash

# why we need network simulation:
# - tests TURN fallback behavior
# - verifies connection reliability
# - simulates real-world conditions

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root (sudo)"
  exit 1
fi

# Function to clean up rules
cleanup() {
    echo "Cleaning up network rules..."
    tc qdisc del dev lo root 2>/dev/null
    iptables -D INPUT -p udp --dport 3478 -j DROP 2>/dev/null
    echo "Network conditions restored to normal"
}

# Ensure cleanup on script exit
trap cleanup EXIT

case "$1" in
    "symmetric-nat")
        echo "Simulating symmetric NAT by blocking direct UDP..."
        iptables -A INPUT -p udp --dport 3478 -j DROP
        ;;
    "latency")
        echo "Adding 100ms latency to localhost..."
        tc qdisc add dev lo root netem delay 100ms
        ;;
    "packet-loss")
        echo "Simulating 10% packet loss..."
        tc qdisc add dev lo root netem loss 10%
        ;;
    "clean")
        cleanup
        exit 0
        ;;
    *)
        echo "Usage: $0 {symmetric-nat|latency|packet-loss|clean}"
        exit 1
        ;;
esac

echo "Network condition applied. Press Ctrl+C to restore normal conditions."
read -r -d '' _ 