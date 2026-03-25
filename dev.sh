#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ---------------------------------------------------------------------------
# dev.sh — Build and run cosmicbizwitch locally
#
# Steps:
#   1. Copy .env.example -> .env if .env doesn't exist
#   2. Create data/ directory if missing
#   3. Build frontend (web/ -> internal/ui/dist/)
#   4. Build Go binary (CGO required for go-libsql)
#   5. Run the binary
# ---------------------------------------------------------------------------

# Step 1: .env setup
if [ ! -f .env ]; then
  echo "==> No .env found, copying from .env.example ..."
  cp .env.example .env
  echo "    Edit .env as needed, then re-run this script."
fi

# Step 2: data directory
mkdir -p data

# Step 3: Build frontend
echo "==> Building frontend ..."
cd web
if [ ! -d node_modules ]; then
  echo "  Installing npm dependencies ..."
  npm install
fi
npm run build
cd "$SCRIPT_DIR"
echo "    Frontend built -> internal/ui/dist/"

# Step 4: Build Go binary
echo "==> Building Go binary ..."
CGO_ENABLED=1 go build -o cosmicbizwitch ./internal/app/cmd/
echo "    Binary built -> ./cosmicbizwitch"

# Step 5: Run
echo "==> Starting cosmicbizwitch ..."
echo ""
# Write PID file so backup.sh can stop the server cleanly
./cosmicbizwitch &
APP_PID=$!
echo $APP_PID > cosmicbizwitch.pid
trap 'rm -f cosmicbizwitch.pid' EXIT
wait $APP_PID
