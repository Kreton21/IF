#!/bin/bash

# ═══════════════════════════════════════════════════════════
# L'Interfilières Festival - Deployment Script
# ═══════════════════════════════════════════════════════════
# This script:
# - Uses local code already present on disk
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

# Step 1: Use local source code
echo -e "${YELLOW}[1/4]${NC} Using local source code (no Git operations)..."
cd "$PROJECT_DIR"
echo -e "${GREEN}✓ Local source ready${NC}"
echo ""

# Step 2: Check Go dependencies
echo -e "${YELLOW}[2/4]${NC} Checking Go dependencies..."
cd "$BACKEND_DIR"

go mod download || {
    echo -e "${RED}✗ Failed to download dependencies${NC}"
    exit 1
}

echo -e "${GREEN}✓ Dependencies ready${NC}"
echo ""

# Step 3: Build backend
echo -e "${YELLOW}[3/4]${NC} Building Go backend..."

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
echo -e "${YELLOW}[4/4]${NC} Checking Docker containers..."

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

# Verify systemd service configuration
if systemctl list-unit-files | grep -q "$SERVICE_NAME"; then
    echo -e "${YELLOW}[4/4]${NC} Checking systemd service configuration..."
    
    if ! sudo grep -q "FRONTEND_DIR=$PROJECT_DIR/frontend" /etc/systemd/system/"$SERVICE_NAME" 2>/dev/null; then
        echo -e "${YELLOW}⚠️  Updating systemd service with correct FRONTEND_DIR...${NC}"
        
        # Check if FRONTEND_DIR line exists but with wrong path
        if sudo grep -q "Environment=\"FRONTEND_DIR=" /etc/systemd/system/"$SERVICE_NAME"; then
            sudo sed -i "s|Environment=\"FRONTEND_DIR=.*\"|Environment=\"FRONTEND_DIR=$PROJECT_DIR/frontend\"|" /etc/systemd/system/"$SERVICE_NAME"
        else
            # Add FRONTEND_DIR after PATH environment variable
            sudo sed -i "/Environment=\"PATH=/a Environment=\"FRONTEND_DIR=$PROJECT_DIR/frontend\"" /etc/systemd/system/"$SERVICE_NAME"
        fi
        
        sudo systemctl daemon-reload
        echo -e "${GREEN}✓ Service configuration updated${NC}"
    else
        echo -e "${GREEN}✓ Service configuration correct${NC}"
    fi
    echo ""
fi

# Step 5: Restart service
echo -e "${YELLOW}[4/4]${NC} Restarting festival service..."

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
