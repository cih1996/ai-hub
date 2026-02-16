#!/bin/bash
set -e

echo "=== Building AI Hub ==="

# Build frontend
echo "[1/3] Building frontend..."
cd web
npm run build
cd ..

# Copy dist to where Go embed expects it
echo "[2/3] Preparing embed..."
rm -rf web/dist/.gitkeep

# Build Go binary
echo "[3/3] Building Go binary..."
CGO_ENABLED=1 go build -o ai-hub .

echo ""
echo "=== Build complete! ==="
echo "Run: ./ai-hub"
echo "Open: http://localhost:8080"
