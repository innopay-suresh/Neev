#!/bin/bash
# start.sh - Automatically detects the correct host IP and starts the server.

echo "Detecting host IP address..."
if command -v ip > /dev/null; then
    # Linux: Get the IP used to route to the internet
    export PUBLIC_IP=$(ip route get 1.1.1.1 | awk -F"src " 'NR==1{split($2,a," ");print a[1]}')
else
    # macOS: Get the primary interface IP
    IFACE=$(route get 1.1.1.1 | grep interface | awk '{print $2}')
    export PUBLIC_IP=$(ipconfig getifaddr $IFACE)
fi

if [ -z "$PUBLIC_IP" ]; then
    echo "Warning: Could not detect IP. Falling back to 127.0.0.1"
    export PUBLIC_IP="127.0.0.1"
fi

echo "Using IP: $PUBLIC_IP"

# Start the docker containers
docker compose up -d
