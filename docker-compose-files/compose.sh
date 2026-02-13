#!/bin/bash

set -euo pipefail

scriptdir=$(cd "$(dirname "$0")"; pwd)
topdir=$(cd "$scriptdir/../"; pwd)

envfile="$scriptdir/.env"

if [[ $(id -u) = 0 ]]; then
  echo "Please do not run $(basename "$0") as root."
  exit 1
fi

gen_secret() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen -r
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  date +%s%N | sha256sum | awk '{print $1}'
}

# Capture environment variables BEFORE any local assignments
PASSED_LOG_LEVEL="${LOG_LEVEL:-}"

MYSQL_ROOT_PASSWORD=""
MYSQL_PASSWORD=""
CONFIGUI_JWT_SECRET=""
LOG_LEVEL=""

if [[ -s "$envfile" ]]; then
  # shellcheck disable=SC1090
  . "$envfile"
fi

# Use passed environment variable if provided, otherwise keep .env value
if [[ -n "${PASSED_LOG_LEVEL:-}" ]]; then
  LOG_LEVEL="$PASSED_LOG_LEVEL"
fi

if [[ -z "${MYSQL_ROOT_PASSWORD:-}" ]]; then
  MYSQL_ROOT_PASSWORD=$(gen_secret)
fi

if [[ -z "${MYSQL_PASSWORD:-}" ]]; then
  MYSQL_PASSWORD=$(gen_secret)
fi

if [[ -z "${CONFIGUI_JWT_SECRET:-}" ]]; then
  CONFIGUI_JWT_SECRET=$(gen_secret)
fi

# Set default LOG_LEVEL only if no value was passed and none exists in .env
if [[ -z "${LOG_LEVEL:-}" && -z "${PASSED_LOG_LEVEL:-}" ]]; then
  LOG_LEVEL="warn"
fi

cat >"$envfile" <<EOF
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}
MYSQL_PASSWORD=${MYSQL_PASSWORD}
CONFIGUI_JWT_SECRET=${CONFIGUI_JWT_SECRET}
LOG_LEVEL=${LOG_LEVEL}
EOF

chmod 600 "$envfile" || true

echo "Using env file: $envfile"

if [[ $# -eq 0 ]]; then
  set -- up -d
fi

compose_cmd="$1"
should_setup_certs=0
case "$compose_cmd" in
  up|start|restart)
    should_setup_certs=1
    ;;
esac

CERT_DIR="${CERT_DIR:-$topdir/.certs}"
CERT="$CERT_DIR/localhost.crt"
KEY="$CERT_DIR/localhost.key"

if [[ "$should_setup_certs" == "1" ]]; then
  if [[ ! -d "$CERT_DIR" ]]; then
    mkdir -p "$CERT_DIR"
  fi
  chmod 755 "$CERT_DIR" || true
  if [[ ! -f "$CERT" || ! -f "$KEY" ]]; then
    echo "Certs not found in $CERT_DIR; running certgen..."
    docker compose --project-directory "$topdir" -f "$scriptdir/docker-compose.yml" --env-file "$envfile" --profile certgen run --rm certgen
  else
    echo "Certs already exist in $CERT_DIR; skipping certgen."
    echo "Modifying certificate permissions for container user (uid:gid 3012:3012)..."
    echo "Target: irc-reference-tools_certs-volume:/certs (localhost.crt, localhost.key)"
    docker run --rm -u 0:0 -v $CERT_DIR:/certs alpine:3.20 sh -c '
    chown 3012:3012 /certs/localhost.key /certs/localhost.crt &&
    chmod 600 /certs/localhost.key &&
    chmod 644 /certs/localhost.crt
  '
    echo "Certificate permission update complete."
  fi
fi

docker compose --project-directory "$topdir" -f "$scriptdir/docker-compose.yml" --env-file "$envfile" "$@"
