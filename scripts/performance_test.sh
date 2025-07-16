#!/bin/bash
# scripts/performance_test.sh

set -e

echo "=== Avaron AI Agent Performance Test ==="
echo

# Check if ab is installed
if ! command -v ab &> /dev/null; then
    echo "Installing Apache Bench..."
    sudo apt-get update && sudo apt-get install -y apache2-utils
fi

# Check if agent is running
if ! curl -s http://localhost:8000/health > /dev/null 2>&1; then
    echo "Error: Agent is not running on localhost:8000"
    exit 1
fi

# Create test payloads
echo '{"prompt": "What is 2+2?"}' > /tmp/simple_payload.json
echo '{"prompt": "Explain quantum computing in simple terms."}' > /tmp/medium_payload.json
echo '{"prompt": "Write a detailed analysis of machine learning algorithms, including supervised learning, unsupervised learning, and reinforcement learning. Include examples and use cases for each."}' > /tmp/large_payload.json

# Test 1: Simple queries
echo "Test 1: Simple queries (100 requests, 5 concurrent)"
ab -n 100 -c 5 -p /tmp/simple_payload.json -T application/json \
   -g /tmp/simple_test.tsv \
   http://localhost:8000/api/v1/agent/query

echo -e "\nTest 2: Medium queries (50 requests, 3 concurrent)"
ab -n 50 -c 3 -p /tmp/medium_payload.json -T application/json \
   -g /tmp/medium_test.tsv \
   http://localhost:8000/api/v1/agent/query

echo -e "\nTest 3: Complex queries (20 requests, 2 concurrent)"
ab -n 20 -c 2 -p /tmp/large_payload.json -T application/json \
   -g /tmp/large_test.tsv \
   http://localhost:8000/api/v1/agent/query

# Test metrics endpoint
echo -e "\nTest 4: Metrics endpoint (1000 requests, 10 concurrent)"
ab -n 1000 -c 10 http://localhost:8000/metrics

echo -e "\nPerformance test completed!"
echo "Results saved in /tmp/*_test.tsv files" 