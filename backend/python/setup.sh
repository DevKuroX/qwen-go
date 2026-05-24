#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "[qwen-go] Installing Python dependencies..."
pip install -r requirements.txt

echo "[qwen-go] Installing Playwright Chromium browser..."
playwright install chromium

echo "[qwen-go] Python setup complete"
