#!/bin/bash

# why we need platform detection:
# - different commands for different os
# - handles both macos and linux
# - provides consistent output format
get_host_ip() {
    case "$(uname -s)" in
        Darwin)
            # Try WiFi first (most common for macOS)
            IP=$(ipconfig getifaddr en0)
            if [ -z "$IP" ]; then
                # Try Ethernet if WiFi not found
                IP=$(ipconfig getifaddr en1)
            fi
            ;;
        Linux)
            # Try to find first non-loopback, non-docker IPv4 address
            IP=$(ip -4 addr show | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | grep -v '^127\.' | grep -v '^172\.' | grep -v '^192\.168\.' | head -n 1)
            ;;
        *)
            echo "Unsupported operating system" >&2
            exit 1
            ;;
    esac

    if [ -z "$IP" ]; then
        echo "Could not determine host IP" >&2
        exit 1
    fi

    echo "$IP"
}

# Output just the IP if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    get_host_ip
fi 