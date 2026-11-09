#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${DRMSONIC_DIR:-/home/drmsonic}"
PORT="${DRMSONIC_PORT:-7779}"
SERVICE="${DRMSONIC_SERVICE:-drmsonic}"
CONTAINER="${DRMSONIC_CONTAINER:-drmsonic}"

log() { printf '\033[1;34m[drmsonic]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[drmsonic]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root: curl -fsSL .../uninstall.sh | sudo bash"

log "stopping ${SERVICE}"
if command -v systemctl >/dev/null 2>&1; then
	systemctl stop "${SERVICE}.service" 2>/dev/null || true
	systemctl disable "${SERVICE}.service" 2>/dev/null || true
	rm -f "/etc/systemd/system/${SERVICE}.service"
	systemctl daemon-reload 2>/dev/null || true
	systemctl reset-failed "${SERVICE}.service" 2>/dev/null || true
fi

if command -v docker >/dev/null 2>&1; then
	docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
	docker rmi drmsonic:latest >/dev/null 2>&1 || true
fi

if command -v ufw >/dev/null 2>&1; then
	ufw delete allow "${PORT}/tcp" >/dev/null 2>&1 || true
fi

if [ -d "$INSTALL_DIR" ]; then
	log "removing $INSTALL_DIR"
	rm -rf -- "$INSTALL_DIR"
fi

for leftover in /opt/drmsonic /usr/local/bin/drmsonic; do
	[ -e "$leftover" ] || continue
	log "removing $leftover"
	rm -rf -- "$leftover"
done

log "wiped"
log "verify: systemctl status ${SERVICE} --no-pager; ls ${INSTALL_DIR}"
