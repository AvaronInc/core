#!/bin/bash
# scripts/setup_harbor.sh

set -e

echo "=== Setting up Harbor Registry ==="
echo

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then 
    echo "Please run with sudo"
    exit 1
fi

# Install dependencies
apt-get update
apt-get install -y wget docker-compose

# Download Harbor
cd /opt
if [ ! -f "harbor-offline-installer-v2.9.0.tgz" ]; then
    echo "Downloading Harbor..."
    wget https://github.com/goharbor/harbor/releases/download/v2.9.0/harbor-offline-installer-v2.9.0.tgz
fi

# Extract
tar xzvf harbor-offline-installer-v2.9.0.tgz
cd harbor

# Configure Harbor
cp harbor.yml.tmpl harbor.yml

# Update configuration
sed -i 's/hostname: reg.mydomain.com/hostname: registry.avaron.local/g' harbor.yml
sed -i 's/https:/#https:/g' harbor.yml
sed -i 's/port: 443/#port: 443/g' harbor.yml
sed -i 's/harbor_admin_password: Harbor12345/harbor_admin_password: AvaronAdmin123!/g' harbor.yml

# Install Harbor
echo "Installing Harbor..."
./install.sh

# Wait for Harbor to start
echo "Waiting for Harbor to start..."
sleep 30

# Create Avaron project
echo "Creating Avaron project..."
curl -X POST "http://registry.avaron.local/api/v2.0/projects" \
  -H "Content-Type: application/json" \
  -u "admin:AvaronAdmin123!" \
  -d '{"project_name": "avaron", "public": true, "metadata": {"public": "true"}}'

echo -e "\nHarbor setup completed!"
echo "Registry URL: http://registry.avaron.local"
echo "Username: admin"
echo "Password: AvaronAdmin123!" 