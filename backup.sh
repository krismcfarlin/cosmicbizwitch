#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="$HOME/Documents/cosmicbizwitch-backups"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DEST="$BACKUP_DIR/backup_$TIMESTAMP"

# ---------------------------------------------------------------------------
# backup.sh — Stop the server, backup data/, restart optional
#
# Usage:
#   ./backup.sh           # backup only (server must not be running, or will be stopped)
#   ./backup.sh --restart # backup then restart the server
# ---------------------------------------------------------------------------

RESTART=false
for arg in "$@"; do
  [[ "$arg" == "--restart" ]] && RESTART=true
done

cd "$SCRIPT_DIR"

# ── Stop any running instance ─────────────────────────────────────────────────
PID_FILE="$SCRIPT_DIR/cosmicbizwitch.pid"

stop_server() {
  # Try pidfile first
  if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE")"
    if kill -0 "$PID" 2>/dev/null; then
      echo "==> Stopping server (pid $PID) ..."
      kill "$PID"
      # Wait up to 10 seconds for graceful shutdown
      for i in $(seq 1 10); do
        kill -0 "$PID" 2>/dev/null || break
        sleep 1
      done
      kill -0 "$PID" 2>/dev/null && kill -9 "$PID" || true
      echo "    Server stopped."
    fi
    rm -f "$PID_FILE"
    return
  fi

  # Fallback: find by process name
  PIDS="$(pgrep -x cosmicbizwitch 2>/dev/null || true)"
  if [ -n "$PIDS" ]; then
    echo "==> Stopping server (pgrep) ..."
    echo "$PIDS" | xargs kill 2>/dev/null || true
    sleep 2
    echo "    Server stopped."
  else
    echo "==> No running server found, proceeding with backup."
  fi
}

stop_server

# ── Backup data/ ─────────────────────────────────────────────────────────────
echo "==> Creating backup at $DEST ..."
mkdir -p "$DEST"

# Copy the data directory preserving structure
cp -R "$SCRIPT_DIR/data/." "$DEST/"

# Also back up .env (contains configured API keys etc.) but skip secrets warning
if [ -f "$SCRIPT_DIR/.env" ]; then
  cp "$SCRIPT_DIR/.env" "$DEST/.env"
  echo "    .env included (contains API keys — keep this backup secure)"
fi

# Summarise what was backed up
FILE_COUNT="$(find "$DEST" -type f | wc -l | tr -d ' ')"
TOTAL_SIZE="$(du -sh "$DEST" | cut -f1)"
echo "    $FILE_COUNT files, $TOTAL_SIZE total"
echo "    Backup complete -> $DEST"

# ── Keep only the 10 most recent backups ─────────────────────────────────────
BACKUP_COUNT="$(ls -1d "$BACKUP_DIR"/backup_* 2>/dev/null | wc -l | tr -d ' ')"
if [ "$BACKUP_COUNT" -gt 10 ]; then
  echo "==> Pruning old backups (keeping 10 most recent) ..."
  ls -1dt "$BACKUP_DIR"/backup_* | tail -n +11 | xargs rm -rf
fi

# ── Optionally restart ────────────────────────────────────────────────────────
if $RESTART; then
  echo "==> Restarting server ..."
  exec "$SCRIPT_DIR/dev.sh"
fi
