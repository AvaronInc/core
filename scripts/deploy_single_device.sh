#!/bin/bash
# scripts/deploy_single_device.sh

set -e

DEVICE_IP=$1
REGISTRY_URL=${2:-"ghcr.io/avaroninc/ai-agent:latest"}

if [ -z "$DEVICE_IP" ]; then
    echo "Usage: $0 <device-ip> [registry-url]"
    exit 1
fi

echo "=== Deploying Avaron AI Agent to $DEVICE_IP ==="
echo

# Create deployment script
cat > /tmp/deploy_agent.sh << 'EOF'
#!/bin/bash
set -e

# Install Docker if needed
if ! command -v docker &> /dev/null; then
    echo "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker $USER
    newgrp docker
fi

# Stop existing agent if running
docker stop avaron-agent 2>/dev/null || true
docker rm avaron-agent 2>/dev/null || true

# Pull latest image
echo "Pulling latest image..."
docker pull REGISTRY_URL_PLACEHOLDER

# Run agent
echo "Starting Avaron AI Agent..."
docker run -d \
  --name avaron-agent \
  --restart unless-stopped \
  -p 8000:8000 \
  -p 11434:11434 \
  -v /var/lib/avaron/models:/root/.ollama \
  -e DEVICE_ID=$(hostname) \
  REGISTRY_URL_PLACEHOLDER

# Wait for startup
echo "Waiting for agent to start..."
for i in {1..30}; do
    if curl -s http://localhost:8000/health > /dev/null 2>&1; then
        echo "Agent is ready!"
        break
    fi
    sleep 2
done

# Test agent
echo "Testing agent..."
curl -s http://localhost:8000/health | json_pp
EOF

# Replace registry URL
sed -i "s|REGISTRY_URL_PLACEHOLDER|$REGISTRY_URL|g" /tmp/deploy_agent.sh

# Copy and execute on remote device
echo "Copying deployment script to device..."
scp /tmp/deploy_agent.sh user@$DEVICE_IP:/tmp/
ssh user@$DEVICE_IP "chmod +x /tmp/deploy_agent.sh && /tmp/deploy_agent.sh"

echo -e "\nDeployment completed!"
echo "Agent URL: http://$DEVICE_IP:8000" 