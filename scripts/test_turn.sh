#!/bin/bash

# why we need comprehensive turn testing:
# - verifies server availability
# - checks credential authentication
# - confirms both stun and turn functionality

echo "Testing TURN server connectivity..."

# Test STUN binding
echo "Testing STUN binding request..."
turnutils_stunclient localhost 3478

# Test TURN allocation
echo "Testing TURN allocation..."
turnutils_uclient -u default -w awestruck-turn-static-auth-key localhost 3478

# Test TURN peer data exchange
echo "Testing TURN peer data exchange..."
turnutils_peer -u default -w awestruck-turn-static-auth-key localhost 3478 