#!/bin/bash

# ═══════════════════════════════════════════════════════════
# L'Interfilières Festival - Deployment Script
# ═══════════════════════════════════════════════════════════
# This script:
# - Pulls latest code from GitHub
# - Rebuilds the Go backend
# - Restarts the festival service
# ═══════════════════════════════════════════════════════════

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"
SERVICE_NAME="festival.service"

echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  L'Interfilières Festival - Deployment${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Step 1: Git pull
echo -e "${YELLOW}[1/5]${NC} Pulling latest code from GitHub..."
cd "$PROJECT_DIR"

# Check if there are uncommitted changes
if [[ -n $(git status -s) ]]; then
    echo -e "${RED}⚠️  Warning: You have uncommitted changes${NC}"
    echo -e "${YELLOW}Stashing local changes...${NC}"
    git stash
    STASHED=1
fi

# Pull latest code
git pull origin main || {
    echo -e "${RED}✗ Failed to pull from GitHub${NC}"
    exit 1
}

if [[ $STASHED -eq 1 ]]; then
    echo -e "${YELLOW}Attempting to restore stashed changes...${NC}"
    git stash pop || echo -e "${YELLOW}⚠️  Stash conflict - resolve manually${NC}"
fi

echo -e "${GREEN}✓ Code updated${NC}"
echo ""

# Step 2: Check Go dependencies
echo -e "${YELLOW}[2/5]${NC} Checking Go dependencies..."
cd "$BACKEND_DIR"

go mod download || {
    echo -e "${RED}✗ Failed to download dependencies${NC}"
    exit 1
}

echo -e "${GREEN}✓ Dependencies ready${NC}"
echo ""

# Step 3: Build backend
echo -e "${YELLOW}[3/5]${NC} Building Go backend..."

# Clean old binary
rm -f festival-server

# Build new binary
go build -o festival-server ./cmd/server || {
    echo -e "${RED}✗ Build failed${NC}"
    exit 1
}

echo -e "${GREEN}✓ Backend built successfully${NC}"
echo ""

# Step 4: Check Docker containers
echo -e "${YELLOW}[4/5]${NC} Checking Docker containers..."

# Make sure postgres and redis are running
cd "$PROJECT_DIR"
docker compose up -d postgres redis

# Wait for health checks
echo "Waiting for containers to be healthy..."
sleep 5

# Verify containers are running
if ! docker ps | grep -q "if-festival-postgres"; then
    echo -e "${RED}✗ PostgreSQL container not running${NC}"
    exit 1
fi

if ! docker ps | grep -q "if-festival-redis"; then
    echo -e "${RED}✗ Redis container not running${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Docker containers running${NC}"
echo ""

# Step 5: Restart service
echo -e "${YELLOW}[5/5]${NC} Restarting festival service..."

# Check if systemd service exists
if systemctl list-unit-files | grep -q "$SERVICE_NAME"; then
    echo "Restarting systemd service..."
    sudo systemctl restart "$SERVICE_NAME"
    
    # Wait a moment for service to start
    sleep 3
    
    # Check service status
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        echo -e "${GREEN}✓ Service restarted successfully${NC}"
        echo ""
        echo -e "${BLUE}Service status:${NC}"
        sudo systemctl status "$SERVICE_NAME" --no-pager -l | head -n 15
    else
        echo -e "${RED}✗ Service failed to start${NC}"
        echo -e "${YELLOW}Showing last 20 log lines:${NC}"
        sudo journalctl -u "$SERVICE_NAME" -n 20 --no-pager
        exit 1
    fi
else
    # No systemd service - try to kill old process and start manually
    echo "No systemd service found - managing manually..."
    
    # Kill old process if running
    OLD_PID=$(lsof -ti:8080 2>/dev/null || true)
    if [[ -n "$OLD_PID" ]]; then
        echo "Stopping old server (PID: $OLD_PID)..."
        kill "$OLD_PID" 2>/dev/null || true
        sleep 2
    fi
    
    # Start new server in background
    cd "$BACKEND_DIR"
    mkdir -p tmp
    nohup ./festival-server > tmp/server.log 2>&1 &
    NEW_PID=$!
    
    echo "Started server with PID: $NEW_PID"
    sleep 2
    
    # Verify it's running
    if ps -p $NEW_PID > /dev/null; then
        echo -e "${GREEN}✓ Server started successfully${NC}"
        echo "Log file: $BACKEND_DIR/tmp/server.log"
    else
        echo -e "${RED}✗ Server failed to start${NC}"
        echo "Check log: $BACKEND_DIR/tmp/server.log"
        exit 1
    fi
fi

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Deployment completed successfully!${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BLUE}URLs:${NC}"
echo -e "  📱 Client: ${GREEN}http://localhost:8080/${NC}"
echo -e "  🔧 Admin:  ${GREEN}http://localhost:8080/admin/${NC}"
echo -e "  📡 API:    ${GREEN}http://localhost:8080/api/v1/${NC}"
echo ""
echo -e "${YELLOW}To view logs:${NC}"
if systemctl list-unit-files | grep -q "$SERVICE_NAME"; then
    echo "  sudo journalctl -u $SERVICE_NAME -f"
else
    echo "  tail -f $BACKEND_DIR/tmp/server.log"
fi
echo ""
