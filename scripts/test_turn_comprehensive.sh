#!/bin/bash

# why we need comprehensive testing:
# - verifies both local and production scenarios
# - tests health check endpoint
# - validates TURN fallback behavior

echo "Starting comprehensive TURN server tests..."

# Test health endpoint
test_health() {
    echo "Testing health endpoint..."
    response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3479/health)
    if [ "$response" = "200" ]; then
        echo "✅ Health check passed"
    else
        echo "❌ Health check failed with status $response"
        return 1
    fi
}

# Test STUN binding
test_stun() {
    echo "Testing STUN binding..."
    if turnutils_stunclient localhost 3478 2>&1 | grep -q "OK"; then
        echo "✅ STUN binding successful"
    else
        echo "❌ STUN binding failed"
        return 1
    fi
}

# Test TURN allocation
test_turn() {
    echo "Testing TURN allocation..."
    if turnutils_uclient -u default -w awestruck-turn-static-auth-key localhost 3478 2>&1 | grep -q "OK"; then
        echo "✅ TURN allocation successful"
    else
        echo "❌ TURN allocation failed"
        return 1
    fi
}

# Test with simulated network conditions
test_with_conditions() {
    local condition=$1
    echo "Testing with $condition..."
    sudo ./scripts/simulate_network.sh "$condition" &
    sleep 2

    # Run tests
    test_health
    test_stun
    test_turn

    # Cleanup
    sudo ./scripts/simulate_network.sh clean
}

# Run basic tests
echo "=== Running basic connectivity tests ==="
test_health || exit 1
test_stun || exit 1
test_turn || exit 1

# Run tests with different network conditions
echo "=== Running tests with network conditions ==="
test_with_conditions "symmetric-nat"
test_with_conditions "latency"
test_with_conditions "packet-loss"

echo "All tests completed!" 