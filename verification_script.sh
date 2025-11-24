#!/bin/bash

echo "🦦 Dock Otter Verification Script"
echo "================================"

# 1. Check if Docker is running
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed or not in PATH"
    exit 1
fi

# 2. Check Container Status
echo -n "🔍 Checking container status... "
if docker ps --format '{{.Names}}' | grep -q "dock-otter"; then
    echo "✅ Running"
else
    echo "❌ Not running"
    echo "   Tip: Run 'docker compose up -d' to start"
    exit 1
fi

# 3. Check Connectivity via Logs (Container is minimal, no wget/curl)
echo "🔍 Checking connectivity via logs..."

# Check for startup connection success
if docker compose logs dock-otter | grep -q "✅ Dokploy connected"; then
    echo "✅ Dokploy connection confirmed in logs:"
    docker compose logs dock-otter | grep "✅ Dokploy connected" | tail -n 1
else
    echo "⚠️  No 'Dokploy connected' log found. Checking for errors..."
    docker compose logs dock-otter | grep "Dokploy connection failed" | tail -n 1
fi

# Check for Pangolin connection
if docker compose logs dock-otter | grep -q "✅ Pangolin API accessible"; then
    echo "✅ Pangolin connection confirmed in logs"
else
    echo "⚠️  No 'Pangolin API accessible' log found."
fi

# 4. Analyze Sync Logic
echo -e "\n📋 Analyzing Sync Behavior..."
LOGS=$(docker compose logs --tail=100 dock-otter)

# Check what it's finding
echo "--- Recent Activity ---"
echo "$LOGS" | grep -E "Found app|Skipping|Processing project|Sync completed" | tail -n 10

if echo "$LOGS" | grep -q "processed=0 skipped=0"; then
    echo -e "\n⚠️  WARNING: Sync completed but processed 0 apps."
    echo "   Possible reasons:"
    echo "   1. No apps are running (Status must be 'done')"
    echo "   2. Apps have no domains configured"
    echo "   3. API key has insufficient permissions"
    echo "   4. Dokploy API is returning empty lists"
fi

# 5. Debug API Response (Run on Host)
echo -e "\n🔍 Debugging Dokploy API Response (Host-side)..."
if [ -f .env ]; then
    set -a
    source .env
    set +a
    
    if [ -z "$DOKPLOY_API_KEY" ] && [ -z "$DOKPLOY_TOKEN" ]; then
        echo "❌ No API Key or Token found in .env"
    else
        echo "🔑 Found credentials in .env"
        echo "📡 Testing /api/project/all..."
        
        # Construct header
        HEADER="Authorization: Bearer $DOKPLOY_TOKEN"
        if [ -n "$DOKPLOY_API_KEY" ]; then
            HEADER="X-API-Key: $DOKPLOY_API_KEY"
        fi
        
        # Curl and show first 1000 chars of structure
        RESPONSE=$(curl -s -H "$HEADER" "$DOKPLOY_URL/api/project/all")
        if [ -n "$RESPONSE" ]; then
            echo "📄 Raw JSON (truncated):"
            echo "$RESPONSE" | cut -c 1-1000
            echo "..."
        else
            echo "❌ No response from API"
            echo "   Trying /api/projects..."
             RESPONSE=$(curl -s -H "$HEADER" "$DOKPLOY_URL/api/projects")
             echo "$RESPONSE" | cut -c 1-1000
        fi
    fi
else
    echo "⚠️  .env file not found, cannot debug API"
fi

echo -e "\n================================"
echo "Verification Complete"
