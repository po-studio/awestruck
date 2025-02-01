#!/bin/sh

# why we need argument validation:
# - ensures required parameters are set
# - provides meaningful error messages
# - prevents silent failures

if [ -z "$PUBLIC_IP" ]; then
    echo "ERROR: PUBLIC_IP environment variable is required"
    exit 1
fi

if [ -z "$USERS" ]; then
    echo "ERROR: USERS environment variable is required"
    exit 1
fi

# Set default values if not provided
TURN_REALM=${TURN_REALM:-"awestruck.io"}
UDP_PORT=${UDP_PORT:-3478}

echo "Starting TURN server with configuration:"
echo "Public IP: $PUBLIC_IP"
echo "UDP Port: $UDP_PORT"
echo "Realm: $TURN_REALM"

exec /app/turn-server \
    -public-ip "$PUBLIC_IP" \
    -port "$UDP_PORT" \
    -users "$USERS" \
    -realm "$TURN_REALM" 