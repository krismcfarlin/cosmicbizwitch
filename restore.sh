#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="$HOME/Documents/cosmicbizwitch-backups"

# ---------------------------------------------------------------------------
# restore.sh — Stop the server, restore data/ from a backup, restart optional
#
# Usage:
#   ./restore.sh                   # restore from most recent backup
#   ./restore.sh backup_20240101_120000  # restore a specific backup
#   ./restore.sh --list            # list available backups
#   ./restore.sh --restart         # restore then restart the server
# ---------------------------------------------------------------------------

RESTART=false
BACKUP_NAME=""

for arg in "$@"; do
  case "$arg" in
    --restart) RESTART=true ;;
    --list)
      echo "Available backups in $BACKUP_DIR:"
      ls -1dt "$BACKUP_DIR"/backup_* 2>/dev/null | while read -r b; do
        SIZE="$(du -sh "$b" | cut -f1)"
        FILES="$(find "$b" -type f | wc -l | tr -d ' ')"
        echo "  $(basename "$b")  ($FILES files, $SIZE)"
      done
      exit 0
      ;;
    --*) echo "Unknown flag: $arg" >&2; exit 1 ;;
    *)   BACKUP_NAME="$arg" ;;
  esac
done

# ── Resolve which backup to use ───────────────────────────────────────────────
if [ -n "$BACKUP_NAME" ]; then
  # Allow bare name (backup_20240101_120000) or full path
  if [[ "$BACKUP_NAME" == /* ]]; then
    SOURCE="$BACKUP_NAME"
  else
    SOURCE="$BACKUP_DIR/$BACKUP_NAME"
  fi
else
  # Default: most recent
  SOURCE="$(ls -1dt "$BACKUP_DIR"/backup_* 2>/dev/null | head -n1 || true)"
  if [ -z "$SOURCE" ]; then
    echo "ERROR: No backups found in $BACKUP_DIR" >&2
    exit 1
  fi
fi

if [ ! -d "$SOURCE" ]; then
  echo "ERROR: Backup not found: $SOURCE" >&2
  exit 1
fi

echo "==> Restoring from: $SOURCE"

# ── Stop any running instance ─────────────────────────────────────────────────
PID_FILE="$SCRIPT_DIR/cosmicbizwitch.pid"

stop_server() {
  if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE")"
    if kill -0 "$PID" 2>/dev/null; then
      echo "==> Stopping server (pid $PID) ..."
      kill "$PID"
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

  PIDS="$(pgrep -x cosmicbizwitch 2>/dev/null || true)"
  if [ -n "$PIDS" ]; then
    echo "==> Stopping server (pgrep) ..."
    echo "$PIDS" | xargs kill 2>/dev/null || true
    sleep 2
    echo "    Server stopped."
  else
    echo "==> No running server found, proceeding with restore."
  fi
}

stop_server

# ── Restore data/ ─────────────────────────────────────────────────────────────
DATA_DIR="$SCRIPT_DIR/data"

echo "==> Restoring data/ ..."
rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"
# Copy everything except .env back into data/
rsync -a --exclude='.env' "$SOURCE/" "$DATA_DIR/"

# ── Restore .env if present in backup ─────────────────────────────────────────
if [ -f "$SOURCE/.env" ]; then
  if [ -f "$SCRIPT_DIR/.env" ]; then
    echo "==> Backing up current .env to .env.pre-restore ..."
    cp "$SCRIPT_DIR/.env" "$SCRIPT_DIR/.env.pre-restore"
  fi
  cp "$SOURCE/.env" "$SCRIPT_DIR/.env"
  echo "    .env restored (API keys restored from backup)"
fi

FILE_COUNT="$(find "$DATA_DIR" -type f | wc -l | tr -d ' ')"
echo "    $FILE_COUNT files restored to $DATA_DIR"
echo "    Restore complete."

# ── Optionally restart ────────────────────────────────────────────────────────
if $RESTART; then
  echo "==> Restarting server ..."
  exec "$SCRIPT_DIR/dev.sh"
fi
