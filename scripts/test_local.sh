#!/bin/bash
# scripts/test_local.sh

set -e

echo "=== Avaron AI Agent Local Test Script ==="
echo

# Check if Ollama is installed
if ! command -v ollama &> /dev/null; then
    echo "Installing Ollama..."
    curl -fsSL https://ollama.com/install.sh | sh
fi

echo "Starting Ollama service..."
ollama serve &
OLLAMA_PID=$!
sleep 5

echo "Pulling Mistral model (this may take a while)..."
ollama pull mistral:7b

echo "Installing Python dependencies..."
pip install -r requirements.txt

echo "Running unit tests..."
pytest tests/ -v

echo "Starting FastAPI server..."
python main.py &
API_PID=$!
sleep 5

echo "Testing API endpoints..."
echo "1. Health check:"
curl -s http://localhost:8000/health | json_pp

echo -e "\n2. Basic query:"
curl -s -X POST http://localhost:8000/api/v1/agent/query \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello, how are you?"}' | json_pp

echo -e "\n3. Metrics check:"
curl -s http://localhost:8000/metrics | head -20

# Cleanup
echo -e "\nCleaning up..."
kill $OLLAMA_PID $API_PID 2>/dev/null || true

echo -e "\nLocal tests completed!" 