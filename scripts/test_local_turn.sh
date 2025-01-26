#!/bin/bash

# why we need local turn testing:
# - verifies turn server functionality
# - tests relay allocation
# - checks authentication

echo "Testing TURN server with forced relay..."

# Test TURN allocation with dummy credentials
turnutils_uclient -v -u awestruck_user -w verySecurePassword1234567890abcdefghijklmnop -y -t -p 3478 -L 127.0.0.1 -X localhost

# Note: -y forces relay usage, -t enables TCP, -L sets local address, -X sets server address 