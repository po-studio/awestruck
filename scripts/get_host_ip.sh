#!/bin/bash

# Try to get IP from various interfaces, preferring non-localhost
# First try en0 (WiFi on Mac)
ip_address=$(ifconfig en0 2>/dev/null | grep "inet " | awk '{print $2}')

# Then try en1 (Ethernet on Mac)
if [ -z "$ip_address" ]; then
    ip_address=$(ifconfig en1 2>/dev/null | grep "inet " | awk '{print $2}')
fi

# Then try any other interface except localhost and docker
if [ -z "$ip_address" ]; then
    ip_address=$(ifconfig | grep "inet " | grep -v "127.0.0.1" | grep -v "172." | grep -v "docker" | awk '{print $2}' | head -n 1)
fi

# If still no IP found, fallback to localhost
if [ -z "$ip_address" ]; then
    ip_address="127.0.0.1"
fi

echo "$ip_address" 
