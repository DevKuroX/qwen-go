#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== Qwen-Go Production Deployment ==="

echo "1. Building frontend static export..."
bash scripts/build-frontend.sh

echo "2. Building Go binary..."
cd backend
go build -o qwen-go ./cmd/qwen-go/

echo "3. Installing Python dependencies..."
./qwen-go python setup

echo "4. Restarting server..."
pkill -f "qwen-go start" 2>/dev/null || true
sleep 1
setsid ./qwen-go start > /tmp/qwen-go.log 2>&1 &
echo "   PID: $(pgrep -f 'qwen-go start' | head -1)"

echo "5. Reloading nginx..."
sudo nginx -t && sudo systemctl reload nginx

echo "=== Deployment complete ==="
echo "API:  http://localhost:1440"
echo "Dash: http://localhost:1441"
