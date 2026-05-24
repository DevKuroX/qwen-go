#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$SCRIPT_DIR/../frontend"
BACKUP_DIR="$SCRIPT_DIR/../.frontend-backup"

cd "$FRONTEND_DIR"

echo "[qwen-go] Building frontend static export..."

# Move API routes and middleware OUTSIDE frontend/ so Next.js doesn't pick them up
mkdir -p "$BACKUP_DIR"
restore_list=""
if [ -d "app/api" ]; then
    mv app/api "$BACKUP_DIR/api"
    restore_list="$restore_list api"
fi
if [ -f "middleware.ts" ]; then
    mv middleware.ts "$BACKUP_DIR/middleware.ts"
    restore_list="$restore_list middleware"
fi

# Build with static export
NEXT_OUTPUT=export npx next build
build_exit=$?

# Restore API routes and middleware
if [ -d "$BACKUP_DIR/api" ]; then
    mv "$BACKUP_DIR/api" app/api
fi
if [ -f "$BACKUP_DIR/middleware.ts" ]; then
    mv "$BACKUP_DIR/middleware.ts" middleware.ts
fi

# Clean backup dir
rmdir "$BACKUP_DIR" 2>/dev/null || true

if [ $build_exit -ne 0 ]; then
    echo "[qwen-go] Frontend build failed"
    exit $build_exit
fi

if [ ! -d "out" ]; then
    echo "[qwen-go] ERROR: out/ directory not created"
    exit 1
fi

# Copy to backend for Go embedding
DASHBOARD_DIR="$SCRIPT_DIR/../backend/internal/server/dashboard"
rm -rf "$DASHBOARD_DIR"
cp -r out "$DASHBOARD_DIR"
echo "[qwen-go] Frontend copied to backend/dashboard/ for Go embedding"

echo "[qwen-go] Frontend static export complete: out/"
