#!/bin/bash
# scripts/build_test.sh

set -e

echo "=== Avaron AI Agent Build Test Script ==="
echo

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed"
    exit 1
fi

# Create buildx builder if not exists
if ! docker buildx ls | grep -q avaron-builder; then
    echo "Creating Docker buildx builder..."
    docker buildx create --name avaron-builder --use
    docker buildx inspect --bootstrap
fi

# Build for current platform
echo "Building Docker image for current platform..."
docker buildx build --platform linux/amd64 -t avaron/ai-agent:test --load .

# Run container
echo "Starting container..."
docker run --rm -d --name avaron-test \
  -p 8000:8000 -p 11434:11434 \
  avaron/ai-agent:test

# Wait for startup
echo "Waiting for services to start..."
for i in {1..60}; do
    if curl -s http://localhost:8000/health > /dev/null 2>&1; then
        echo "Service is ready!"
        break
    fi
    echo -n "."
    sleep 2
done
echo

# Test endpoints
echo "Testing API endpoints..."
echo "1. Health check:"
curl -s http://localhost:8000/health | json_pp || echo "Failed"

echo -e "\n2. API query:"
curl -s -X POST http://localhost:8000/api/v1/agent/query \
  -H "Content-Type: application/json" \
  -d '{"prompt": "What is Docker?"}' | json_pp || echo "Failed"

echo -e "\n3. Metrics:"
curl -s http://localhost:8000/metrics | grep -E "(agent_requests_total|agent_request_duration)" || echo "No metrics found"

# Show logs
echo -e "\nContainer logs:"
docker logs avaron-test --tail 20

# Cleanup
echo -e "\nCleaning up..."
docker stop avaron-test

echo -e "\nBuild test completed!" 