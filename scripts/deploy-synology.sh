#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# deploy-synology.sh — build and push wyze-smash-deck image to Synology NAS
#
# Usage:
#   ./scripts/deploy-synology.sh
#
# Prerequisites:
#   1. Set SYNOLOGY_HOST in your shell or below (e.g. "192.168.1.100" or NAS hostname)
#   2. Enable SSH on your Synology (Control Panel → Terminal & SNMP → Enable SSH)
#   3. Enable Docker registry on Synology, OR use the image export method (default below)
#   4. Make sure your Synology user has permission to run docker commands
#      (add user to the "docker" group in Synology DSM)
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

SYNOLOGY_HOST="${SYNOLOGY_HOST:-}"
SYNOLOGY_USER="${SYNOLOGY_USER:-admin}"
IMAGE_NAME="wyze-smash-deck"
IMAGE_TAG="latest"
DATA_DIR="${SYNOLOGY_DATA_DIR:-/volume1/docker/wyze-smash-deck/data}"

if [[ -z "$SYNOLOGY_HOST" ]]; then
  echo "ERROR: Set SYNOLOGY_HOST before running this script."
  echo "  export SYNOLOGY_HOST=192.168.1.XXX"
  exit 1
fi

cd "$(dirname "$0")/.."

echo "=== Wyze Smash Deck — Synology Deploy ==="
echo "→ Target: ${SYNOLOGY_USER}@${SYNOLOGY_HOST}"
echo ""

# ── 1. Build the image locally ───────────────────────────────────────────────
echo "→ Building Docker image (multi-stage)..."
docker build --platform linux/amd64 -t "${IMAGE_NAME}:${IMAGE_TAG}" .
echo "  ✓ Image built"

# ── 2. Export and transfer ────────────────────────────────────────────────────
echo "→ Exporting image..."
docker save "${IMAGE_NAME}:${IMAGE_TAG}" | gzip > /tmp/${IMAGE_NAME}.tar.gz
echo "  ✓ Exported to /tmp/${IMAGE_NAME}.tar.gz"

echo "→ Copying to Synology..."
scp /tmp/${IMAGE_NAME}.tar.gz "${SYNOLOGY_USER}@${SYNOLOGY_HOST}:/tmp/${IMAGE_NAME}.tar.gz"
echo "  ✓ Transferred"

# ── 3. Load image on Synology and (re)start the container ────────────────────
echo "→ Loading image and restarting container on Synology..."
ssh "${SYNOLOGY_USER}@${SYNOLOGY_HOST}" bash << REMOTE
  set -e
  echo "  Loading image..."
  docker load < /tmp/${IMAGE_NAME}.tar.gz
  rm /tmp/${IMAGE_NAME}.tar.gz

  echo "  Ensuring data directory exists: ${DATA_DIR}"
  mkdir -p "${DATA_DIR}"

  echo "  Stopping existing container (if any)..."
  docker stop ${IMAGE_NAME} 2>/dev/null || true
  docker rm   ${IMAGE_NAME} 2>/dev/null || true

  echo "  Starting container..."
  docker run -d \
    --name ${IMAGE_NAME} \
    --restart unless-stopped \
    -p 8082:8082 \
    -v "${DATA_DIR}:/app/data" \
    --health-cmd="wget -qO- http://localhost:8082/api/health || exit 1" \
    --health-interval=30s \
    --health-timeout=5s \
    --health-retries=3 \
    ${IMAGE_NAME}:${IMAGE_TAG}

  echo ""
  docker ps --filter name=${IMAGE_NAME} --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
REMOTE

rm /tmp/${IMAGE_NAME}.tar.gz

echo ""
echo "✓ Deployed! Open http://${SYNOLOGY_HOST}:8082 in your browser."
echo ""
echo "To view logs: ssh ${SYNOLOGY_USER}@${SYNOLOGY_HOST} docker logs -f ${IMAGE_NAME}"
