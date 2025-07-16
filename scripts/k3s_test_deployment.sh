#!/bin/bash
# scripts/k3s_test_deployment.sh

set -e

echo "=== K3s Deployment Test ==="
echo

# Install K3s if not present
if ! command -v k3s &> /dev/null; then
    echo "Installing K3s..."
    curl -sfL https://get.k3s.io | sh -
    
    # Wait for K3s to be ready
    echo "Waiting for K3s to start..."
    sleep 30
fi

# Export kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Create namespace
echo "Creating Avaron namespace..."
kubectl create namespace avaron --dry-run=client -o yaml | kubectl apply -f -

# Apply configurations
echo "Deploying Avaron AI Agent..."
kubectl apply -f k8s/

# Wait for deployment
echo "Waiting for pods to be ready..."
kubectl wait --for=condition=ready pod -l app=ai-agent -n avaron --timeout=300s

# Check status
echo -e "\nDeployment status:"
kubectl get pods -n avaron
kubectl get svc -n avaron

# Test the service
echo -e "\nTesting agent service..."
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
NODE_PORT=$(kubectl get svc avaron-agent-service -n avaron -o jsonpath='{.spec.ports[?(@.name=="api")].nodePort}')

if [ ! -z "$NODE_PORT" ]; then
    echo "Testing agent at http://$NODE_IP:$NODE_PORT"
    curl -s http://$NODE_IP:$NODE_PORT/health | json_pp
fi

echo -e "\nK3s deployment test completed!" 