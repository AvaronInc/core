#!/bin/bash
# Quick start script for Avaron AI Agent

set -e

echo "=== Avaron AI Agent Quick Start ==="
echo

# Detect OS
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="linux"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
else
    echo "Unsupported OS: $OSTYPE"
    exit 1
fi

echo "Detected OS: $OS"

# Check dependencies
echo "Checking dependencies..."
deps_missing=false

if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found"
    deps_missing=true
else
    echo "✅ Docker found"
fi

if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose not found"
    deps_missing=true
else
    echo "✅ Docker Compose found"
fi

if [ "$deps_missing" = true ]; then
    echo -e "\nPlease install missing dependencies and run again."
    exit 1
fi

# Start services
echo -e "\nStarting Avaron AI Agent..."
docker-compose -f docker-compose.test.yml up -d

# Wait for services
echo "Waiting for services to start..."
sleep 10

# Check health
echo -e "\nChecking service health..."
if curl -s http://localhost:8000/health > /dev/null 2>&1; then
    echo "✅ AI Agent is healthy"
else
    echo "❌ AI Agent health check failed"
fi

if curl -s http://localhost:9090 > /dev/null 2>&1; then
    echo "✅ Prometheus is running"
else
    echo "❌ Prometheus not accessible"
fi

if curl -s http://localhost:3000 > /dev/null 2>&1; then
    echo "✅ Grafana is running"
else
    echo "❌ Grafana not accessible"
fi

echo -e "\n=== Quick Start Complete ==="
echo "AI Agent API: http://localhost:8000"
echo "Prometheus: http://localhost:9090"
echo "Grafana: http://localhost:3000 (admin/admin)"
echo -e "\nTo stop all services: docker-compose -f docker-compose.test.yml down" 