#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${DRMSONIC_REPO:-https://github.com/dragonstracker/drmsonic.git}"
REPO_REF="${DRMSONIC_REF:-main}"
INSTALL_DIR="${DRMSONIC_DIR:-/opt/drmsonic}"
PORT="${DRMSONIC_PORT:-1234}"
CONTAINER="${DRMSONIC_CONTAINER:-drmsonic}"
IMAGE="drmsonic:latest"

log() { printf '\033[1;34m[drmsonic]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[drmsonic]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root (sudo bash install.sh)"

if ! command -v docker >/dev/null 2>&1; then
	log "installing docker engine"
	curl -fsSL https://get.docker.com | sh
	systemctl enable --now docker
fi
command -v docker >/dev/null 2>&1 || die "docker installation failed"

if ! docker info >/dev/null 2>&1; then
	systemctl start docker
fi

if [ -d "$INSTALL_DIR/.git" ]; then
	log "updating $INSTALL_DIR"
	git -C "$INSTALL_DIR" fetch --depth 1 origin "$REPO_REF"
	git -C "$INSTALL_DIR" reset --hard FETCH_HEAD
else
	log "cloning $REPO_URL into $INSTALL_DIR"
	rm -rf "$INSTALL_DIR"
	git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" "$INSTALL_DIR"
fi

SRC="$INSTALL_DIR"
[ -f "$SRC/Dockerfile" ] || SRC="$INSTALL_DIR/drmsonic"
[ -f "$SRC/Dockerfile" ] || die "no Dockerfile under $INSTALL_DIR"

log "building image"
docker build -q -t "$IMAGE" "$SRC"

log "starting container $CONTAINER on port $PORT"
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d \
	--name "$CONTAINER" \
	--restart unless-stopped \
	-p "$PORT:1234" \
	-v "$SRC/providers:/app/providers" \
	"$IMAGE" \
	-p 1234 -b 0.0.0.0 -providers providers -v 1 >/dev/null

cat >/etc/systemd/system/drmsonic.service <<UNIT
[Unit]
Description=DRMSONIC
After=docker.service network-online.target
Requires=docker.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/docker start $CONTAINER
ExecStop=/usr/bin/docker stop $CONTAINER
ExecReload=/usr/bin/docker restart $CONTAINER
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable drmsonic.service >/dev/null 2>&1 || true

if command -v ufw >/dev/null 2>&1; then
	ufw allow "$PORT"/tcp >/dev/null 2>&1 || true
fi

PUBLIC_IP="$(curl -fsS --max-time 4 https://api.ipify.org 2>/dev/null || true)"
log "service is up"
log "panel: http://${PUBLIC_IP:-<server-ip>}:$PORT"
log "logs:  docker logs -f $CONTAINER"
log "restart: systemctl restart drmsonic"
