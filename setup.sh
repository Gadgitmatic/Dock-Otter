#!/bin/bash

# Dock Otter Setup Script - Ensures compatibility across different environments

set -e

echo "🦦 Dock Otter Setup Script"
echo "=========================="

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Docker Compose is available
if docker compose version &> /dev/null; then
    COMPOSE_CMD="docker compose"
elif docker-compose --version &> /dev/null; then
    COMPOSE_CMD="docker-compose"
else
    echo "❌ Docker Compose is not available. Please install Docker Compose."
    exit 1
fi

echo "✅ Using: $COMPOSE_CMD"

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file from template..."
    cp .env.example .env
    echo "⚠️  Please edit .env file with your API keys before continuing!"
    echo "   Required: DOKPLOY_API_KEY and PANGOLIN_TOKEN"
    exit 0
fi

# Check if required networks exist
NETWORKS_EXIST=true

for network in shared-proxy dokploy-network pangolin; do
    if ! docker network ls | grep -q "$network"; then
        echo "⚠️  Network '$network' not found"
        NETWORKS_EXIST=false
    fi
done

# Determine which compose file to use
if [ "$NETWORKS_EXIST" = true ]; then
    echo "✅ All required networks found - using standard deployment"
    COMPOSE_FILE="docker-compose.yml"
else
    echo "⚠️  Some networks missing - using fallback mode with mock services"
    COMPOSE_FILE="docker-compose.yml -f docker-compose.fallback.yml"
fi

# Create logs directory
mkdir -p logs

# Build and start
echo "🚀 Building and starting Dock Otter..."
$COMPOSE_CMD -f $COMPOSE_FILE up -d --build

echo ""
echo "✅ Dock Otter is starting up!"
echo ""
echo "📊 Check status:"
echo "   $COMPOSE_CMD logs -f dock-otter"
echo ""
echo "🏥 Health check:"
echo "   curl http://localhost:8080/health"
echo ""
echo "🛑 To stop:"
echo "   $COMPOSE_CMD down"