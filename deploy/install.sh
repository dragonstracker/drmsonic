#!/usr/bin/env bash
set -euo pipefail

REPO="${DRMSONIC_REPO:-dragonstracker/drmsonic}"
TAG="${DRMSONIC_TAG:-latest}"
INSTALL_DIR="${DRMSONIC_DIR:-/home/drmsonic}"
PORT="${DRMSONIC_PORT:-7779}"
BIND="${DRMSONIC_BIND:-0.0.0.0}"
SERVICE="${DRMSONIC_SERVICE:-drmsonic}"
BIN_NAME="drmsonic"

log() { printf '\033[1;34m[drmsonic]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[drmsonic]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root: curl -fsSL .../install.sh | sudo bash"

ARCH="$(uname -m)"
case "$ARCH" in
	x86_64|amd64) ASSET="drmsonic-linux-amd64" ;;
	aarch64|arm64) ASSET="drmsonic-linux-arm64" ;;
	*) die "unsupported arch: $ARCH" ;;
esac

if [ "$TAG" = "latest" ]; then
	URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
else
	URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
fi

UPGRADE=0
if [ -x "${INSTALL_DIR}/${BIN_NAME}" ] || systemctl cat "${SERVICE}.service" >/dev/null 2>&1; then
	UPGRADE=1
fi

if [ "$UPGRADE" -eq 1 ]; then
	log "upgrade — stopping ${SERVICE}"
	systemctl stop "${SERVICE}.service" 2>/dev/null || true
else
	log "fresh install into ${INSTALL_DIR}"
fi

mkdir -p "$INSTALL_DIR/providers" "$INSTALL_DIR/scripts" "$INSTALL_DIR/logos"
cd "$INSTALL_DIR"

TMP="${BIN_NAME}.new"
log "downloading $URL"
if command -v curl >/dev/null 2>&1; then
	curl -fL --retry 3 -o "$TMP" "$URL"
elif command -v wget >/dev/null 2>&1; then
	wget -O "$TMP" "$URL"
else
	die "need curl or wget"
fi
chmod +x "$TMP"
mv -f "$TMP" "$BIN_NAME"

if [ ! -f providers/example.json ]; then
	cat >providers/example.json <<'JSON'
{
	"Id": "example",
	"Name": "example",
	"RunningMode": "internalremuxer",
	"OutputMode": "directhls",
	"NetworkScope": "script,manifest,media",
	"Headers": [],
	"UserAgent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"HttpGetRetries": 2,
	"StallDetectTimeout": 20,
	"RestartDelay": 5,
	"PlaylistDuration": 30,
	"NbAnnouncedFragments": 6,
	"Streams": []
}
JSON
fi

cat >"/etc/systemd/system/${SERVICE}.service" <<UNIT
[Unit]
Description=DRMSONIC
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BIN_NAME} -p ${PORT} -b ${BIND} -providers providers -v 1
Restart=always
RestartSec=3
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now "${SERVICE}.service"
systemctl restart "${SERVICE}.service"

if command -v ufw >/dev/null 2>&1; then
	ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
fi

PUBLIC_IP="$(curl -fsS --max-time 4 https://api.ipify.org 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')"
if [ "$UPGRADE" -eq 1 ]; then
	log "upgraded"
else
	log "installed"
fi
log "panel: http://${PUBLIC_IP:-<server-ip>}:${PORT}"
log "status: systemctl status ${SERVICE} --no-pager"
log "logs:   journalctl -u ${SERVICE} -f"
