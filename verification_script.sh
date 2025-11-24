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

# 3. Check Internal Connectivity
echo "🔍 Checking internal network connectivity..."

echo -n "   • Dokploy (http://dokploy:3000)... "
if docker exec dock-otter wget -q --spider http://dokploy:3000/api/project/all 2>/dev/null; then
    echo "✅ Connected"
else
    echo "❌ Failed"
fi

echo -n "   • Pangolin (http://pangolin:3001)... "
if docker exec dock-otter wget -q --spider http://pangolin:3001/v1/docs 2>/dev/null; then
    echo "✅ Connected"
else
    echo "❌ Failed"
fi

# 4. Check Logs for Sync Status
echo -e "\n📋 Checking recent logs for sync activity..."
LOGS=$(docker compose logs --tail=50 dock-otter)

if echo "$LOGS" | grep -q "✅ Sync completed"; then
    echo "✅ Found successful sync logs"
    # Extract stats
    echo "$LOGS" | grep "✅ Sync completed" | tail -n 1
elif echo "$LOGS" | grep -q "❌"; then
    echo "⚠️  Found errors in logs:"
    echo "$LOGS" | grep "❌" | tail -n 3
else
    echo "ℹ️  No sync completion logs found recently. Current logs:"
    echo "$LOGS" | tail -n 5
fi

echo -e "\n================================"
echo "Verification Complete"
