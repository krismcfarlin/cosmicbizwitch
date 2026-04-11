#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# deploy.sh — Deploy cosmicbizwitch to server.cosmicbizwitch.com
#
# What this does:
#   1. Stops the running service gracefully
#   2. Backs up /opt/cosmicbizwitch/data/ to /opt/cosmicbizwitch/backups/
#   3. Rsyncs source code to /opt/cosmicbizwitch/src/
#      (excludes data/, *.db, .env — data is NEVER touched by rsync)
#   4. Builds the binary on the server (CGO required for go-libsql)
#   5. Atomically swaps the binary (keeps .prev for rollback)
#   6. Restarts the service
#
# Prerequisites on server (one-time setup):
#   - Go installed at /usr/local/go/bin/go (or in PATH)
#   - gcc installed (for CGO)
#   - systemd service "cosmicbizwitch" configured (see cosmicbizwitch.service)
#   - /opt/cosmicbizwitch/data/ exists and contains your databases
#   - /opt/cosmicbizwitch/.env exists with your env vars
#   - /opt/cosmicbizwitch/backups/ directory exists (created automatically)
# ---------------------------------------------------------------------------

# Always run from the project root (directory containing this script).
cd "$(dirname "$0")"

SERVER="${DEPLOY_SERVER:-159.223.157.70}"
REMOTE_DIR="/opt/cosmicbizwitch"
SERVICE="cosmicbizwitch"
REMOTE_SRC="$REMOTE_DIR/src"

# Use a single SSH ControlMaster socket so all connections reuse one session.
SOCKET="/tmp/deploy-ssh-$$"
SSH="ssh -o ControlMaster=auto -o ControlPath=$SOCKET -o ControlPersist=120"

# Open the master connection once.
$SSH -fN "root@$SERVER"

cleanup() { ssh -O exit -o ControlPath="$SOCKET" "root@$SERVER" 2>/dev/null || true; }
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Step 1: Stop the service
# ---------------------------------------------------------------------------
echo "==> Stopping service ..."
$SSH "root@$SERVER" bash <<'ENDSSH'
  set -euo pipefail
  if systemctl is-active --quiet cosmicbizwitch; then
    systemctl stop cosmicbizwitch
    echo "  Service stopped."
  else
    echo "  Service was not running."
  fi
ENDSSH

# ---------------------------------------------------------------------------
# Step 2: Back up data
# ---------------------------------------------------------------------------
echo "==> Backing up data ..."
$SSH "root@$SERVER" bash <<'ENDSSH'
  set -euo pipefail
  BACKUP_DIR="/opt/cosmicbizwitch/backups"
  DATA_DIR="/opt/cosmicbizwitch/data"
  TIMESTAMP=$(date +"%Y%m%d-%H%M%S")
  BACKUP_FILE="$BACKUP_DIR/data-$TIMESTAMP.tar.gz"

  mkdir -p "$BACKUP_DIR"

  if [ -d "$DATA_DIR" ] && [ "$(ls -A "$DATA_DIR" 2>/dev/null)" ]; then
    tar -czf "$BACKUP_FILE" -C /opt/cosmicbizwitch data/
    echo "  Backup created: $BACKUP_FILE"
    # Keep last 10 backups, remove older ones
    ls -t "$BACKUP_DIR"/data-*.tar.gz | tail -n +11 | xargs -r rm --
    echo "  Old backups pruned (keeping 10 most recent)."
  else
    echo "  No data to back up (data/ is empty or missing)."
  fi
ENDSSH

# ---------------------------------------------------------------------------
# Step 3: Sync source code (data is excluded)
# ---------------------------------------------------------------------------
echo "==> Syncing source to $SERVER:$REMOTE_SRC ..."
rsync -av --delete \
  -e "ssh -o ControlMaster=auto -o ControlPath=$SOCKET -o ControlPersist=120" \
  --exclude='.git/' \
  --exclude='data/' \
  --exclude='*.db' \
  --exclude='*.db-shm' \
  --exclude='*.db-wal' \
  --exclude='.env' \
  --exclude='.env.*' \
  --exclude='deploy.sh' \
  --exclude='web/node_modules/' \
  --exclude='*.service' \
  --exclude='backups/' \
  . "root@$SERVER:$REMOTE_SRC/"

# ---------------------------------------------------------------------------
# Step 3b: Install system dependencies
# ---------------------------------------------------------------------------
echo "==> Installing system dependencies ..."
$SSH "root@$SERVER" bash <<'ENDSSH'
  if ! command -v rsvg-convert &>/dev/null; then
    apt-get install -y -q librsvg2-bin
    echo "  rsvg-convert installed."
  else
    echo "  rsvg-convert already present: $(rsvg-convert --version 2>&1 | head -1)"
  fi
ENDSSH

# ---------------------------------------------------------------------------
# Step 4: Build on server
# ---------------------------------------------------------------------------
echo "==> Building on server (CGO, linux/amd64) ..."
$SSH "root@$SERVER" bash <<'ENDSSH'
  set -euo pipefail
  export PATH=/usr/local/go/bin:$PATH
  cd /opt/cosmicbizwitch/src

  echo "  go version: $(go version)"

  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /opt/cosmicbizwitch/cosmicbizwitch.new \
    ./internal/app/cmd/

  echo "  Build successful."
ENDSSH

# ---------------------------------------------------------------------------
# Step 5: Swap binary
# ---------------------------------------------------------------------------
echo "==> Swapping binary ..."
$SSH "root@$SERVER" bash <<'ENDSSH'
  set -euo pipefail
  if [ -f /opt/cosmicbizwitch/cosmicbizwitch ]; then
    cp /opt/cosmicbizwitch/cosmicbizwitch /opt/cosmicbizwitch/cosmicbizwitch.prev
  fi
  mv /opt/cosmicbizwitch/cosmicbizwitch.new /opt/cosmicbizwitch/cosmicbizwitch
  chmod +x /opt/cosmicbizwitch/cosmicbizwitch
  chown cosmicbizwitch:cosmicbizwitch /opt/cosmicbizwitch/cosmicbizwitch
ENDSSH

# ---------------------------------------------------------------------------
# Step 6: Start service
# ---------------------------------------------------------------------------
echo "==> Starting service ..."
$SSH "root@$SERVER" "systemctl start $SERVICE && systemctl is-active $SERVICE"

echo ""
echo "==> Deploy complete. https://server.cosmicbizwitch.com"
