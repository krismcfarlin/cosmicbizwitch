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

SERVER="${DEPLOY_SERVER:-server.cosmicbizwitch.com}"
REMOTE_DIR="/opt/cosmicbizwitch"
SERVICE="cosmicbizwitch"
REMOTE_SRC="$REMOTE_DIR/src"

# ---------------------------------------------------------------------------
# Step 1: Stop the service
# ---------------------------------------------------------------------------
echo "==> Stopping service ..."
ssh "root@$SERVER" bash <<'ENDSSH'
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
ssh "root@$SERVER" bash <<'ENDSSH'
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
# Step 4: Build on server
# ---------------------------------------------------------------------------
echo "==> Building on server (CGO, linux/amd64) ..."
ssh "root@$SERVER" bash <<'ENDSSH'
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
ssh "root@$SERVER" bash <<'ENDSSH'
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
ssh "root@$SERVER" "systemctl start $SERVICE && systemctl is-active $SERVICE"

echo ""
echo "==> Deploy complete. https://server.cosmicbizwitch.com"
