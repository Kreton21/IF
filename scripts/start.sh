#!/bin/bash
# Start all IF Festival services (PostgreSQL, Redis, Go backend)
# Usage: ./scripts/start.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}🎵 IF Festival - Starting all services...${NC}\n"

# ── 1. Start Docker containers (PostgreSQL + Redis) ──
echo -e "${YELLOW}[1/3] Starting Docker containers...${NC}"
cd "$PROJECT_DIR"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}✗ Docker is not installed.${NC}"
    exit 1
fi

docker compose up -d postgres redis 2>&1 | tail -5

# Wait for PostgreSQL to be healthy
echo -ne "${YELLOW}      Waiting for PostgreSQL..."
for i in $(seq 1 30); do
    if docker exec if-festival-postgres pg_isready -U festival -d festival_db &> /dev/null; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e " ${RED}timeout${NC}"
        exit 1
    fi
    sleep 1
    echo -n "."
done

# Wait for Redis to be healthy
echo -ne "${YELLOW}      Waiting for Redis..."
for i in $(seq 1 15); do
    if docker exec if-festival-redis redis-cli ping &> /dev/null; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 15 ]; then
        echo -e " ${RED}timeout${NC}"
        exit 1
    fi
    sleep 1
    echo -n "."
done

echo -e "${GREEN}      ✓ Docker containers running${NC}\n"

# ── 2. Build the Go backend ──
echo -e "${YELLOW}[2/3] Building backend...${NC}"
cd "$PROJECT_DIR/backend"
go build -o ./tmp/server ./cmd/server/main.go
echo -e "${GREEN}      ✓ Build successful${NC}\n"

# ── 3. Start the backend server ──
echo -e "${YELLOW}[3/3] Starting backend server...${NC}"

# Kill any existing instance on port 8080
if lsof -ti:8080 &> /dev/null; then
    echo -e "      Stopping existing server on port 8080..."
    kill $(lsof -ti:8080) 2>/dev/null || true
    sleep 1
fi

cd "$PROJECT_DIR/backend"
nohup ./tmp/server > ./tmp/server.log 2>&1 &
SERVER_PID=$!

# Wait for the server to respond
sleep 2
if kill -0 "$SERVER_PID" 2>/dev/null; then
    echo -e "${GREEN}      ✓ Backend running (PID: $SERVER_PID)${NC}\n"
else
    echo -e "${RED}      ✗ Backend failed to start. Check ./backend/tmp/server.log${NC}"
    exit 1
fi

# ── Summary ──
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}  All services started successfully!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "  🌐  App:    ${CYAN}http://localhost:8080${NC}"
echo -e "  🔧  Admin:  ${CYAN}http://localhost:8080/admin${NC}"
echo -e "  📋  Logs:   ${CYAN}tail -f backend/tmp/server.log${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
