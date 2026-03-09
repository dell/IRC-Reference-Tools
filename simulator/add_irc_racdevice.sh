#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://localhost:443}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin}"

IRC_IP="${IRC_IP:-100.71.133.166}"
IRC_HOST="${IRC_HOST:-${IRC_IP}:8456}"
IRC_USER="${IRC_USER:-root}"
IRC_PASS="${IRC_PASS:-calvin}"

RACDEVICE_USER="${RACDEVICE_USER:-root}"
RACDEVICE_PASS="${RACDEVICE_PASS:-calvin}"
RACDEVICE_SYSTEMTYPEDESC="${RACDEVICE_SYSTEMTYPEDESC:-racdevice}"
RACDEVICE_TIMEOUT="${RACDEVICE_TIMEOUT:-300}"
RACDEVICE_FORCEFUL="${RACDEVICE_FORCEFUL:-true}"

RACDEVICE_IP="${RACDEVICE_IP:-100.71.133.166}"
RACDEVICE_PORT_START="${RACDEVICE_PORT_START:-50001}"
RACDEVICE_PORT_END="${RACDEVICE_PORT_END:-}"
RACDEVICE_PORT_COUNT="${RACDEVICE_PORT_COUNT:-10}"
DEVICE_PORT_MAP="${DEVICE_PORT_MAP:-}"
AUTO_PORT_MAP_FROM_PROCESS="${AUTO_PORT_MAP_FROM_PROCESS:-true}"

ASSOCIATE_EACH_IRC="${ASSOCIATE_EACH_IRC:-false}"

DISTRIBUTE_RACDEVICES="${DISTRIBUTE_RACDEVICES:-false}"
RANDOMIZE_RACDEVICES="${RANDOMIZE_RACDEVICES:-false}"

CURL="curl -sS -k"

usage() {
  echo "Usage:"
  echo "  bash $0"
  echo "  bash $0 IRC <start-end> RACDEVICE <start-end>"
  echo "Examples:"
  echo "  bash $0 IRC 8443-8444 RACDEVICE 50001-50010"
  echo "Env vars: BASE_URL, ADMIN_USER, ADMIN_PASS, IRC_IP, IRC_USER, IRC_PASS, RACDEVICE_IP, RACDEVICE_USER, RACDEVICE_PASS, RACDEVICE_TIMEOUT, RACDEVICE_FORCEFUL, DEVICE_PORT_MAP"
  echo "          AUTO_PORT_MAP_FROM_PROCESS=true auto-reads -portMap from running simulator/racdevice process when DEVICE_PORT_MAP is empty."
  echo "Behavior: creates all IRC services in range; associates rack-device service-items to the FIRST IRC by default."
  echo "          Set ASSOCIATE_EACH_IRC=true to associate ALL rack devices to EACH IRC."
  echo "          Set DISTRIBUTE_RACDEVICES=true to distribute rack devices evenly across all IRCs (round-robin)."
  echo "          Set RANDOMIZE_RACDEVICES=true to randomly assign each service-item to one IRC from the range."
}

parse_range() {
  local label="$1"
  local r="$2"
  if [[ "${r}" != *-* ]]; then
    echo "ERROR: ${label} range must be start-end (got: ${r})" >&2
    exit 1
  fi
  local start="${r%-*}"
  local end="${r#*-}"
  if ! [[ "${start}" =~ ^[0-9]+$ && "${end}" =~ ^[0-9]+$ ]]; then
    echo "ERROR: ${label} range must be numeric (got: ${r})" >&2
    exit 1
  fi
  if (( start > end )); then
    echo "ERROR: ${label} range start must be <= end (got: ${r})" >&2
    exit 1
  fi
  echo "${start} ${end}"
}

IRC_HOSTS="${IRC_HOSTS-}"
RACDEVICE_HOSTS="${RACDEVICE_HOSTS-}"

if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "${#}" -gt 0 ]]; then
  if [[ "${#}" -ne 4 ]]; then
    usage
    exit 1
  fi
  key1="${1,,}"
  range1="$2"
  key2="${3,,}"
  range2="$4"

  if [[ "${key1}" != "irc" || "${key2}" != "racdevice" ]]; then
    usage
    exit 1
  fi

  read -r ircStart ircEnd < <(parse_range "IRC" "${range1}")
  read -r racStart racEnd < <(parse_range "RACDEVICE" "${range2}")

  IRC_HOSTS=""
  for ((p=ircStart; p<=ircEnd; p++)); do
    IRC_HOSTS+="${IRC_IP}:${p}"$'\n'
  done

  RACDEVICE_HOSTS=""
  for ((p=racStart; p<=racEnd; p++)); do
    RACDEVICE_HOSTS+="${RACDEVICE_IP}:${p}"$'\n'
  done
fi

if [[ -z "${IRC_HOSTS}" ]]; then
  IRC_HOSTS="${IRC_HOST}"$'\n'
fi

urlencode_path_segment() {
  local s="$1"
  s="${s//:/%3A}"
  echo -n "$s"
}

TOKEN="$($CURL -X POST "${BASE_URL}/api/v1/GetToken" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
)"

if [[ -z "${TOKEN}" ]]; then
  echo "ERROR: failed to obtain JWT token"
  exit 1
fi

AUTH_HEADER="Authorization: Bearer ${TOKEN}"

firstIrc=""
while read -r ircHost; do
  [[ -z "${ircHost}" ]] && continue
  if [[ -z "${firstIrc}" ]]; then
    firstIrc="${ircHost}"
  fi
  $CURL -X POST "${BASE_URL}/api/v1/irc" \
    -H "Content-Type: application/json" \
    -H "${AUTH_HEADER}" \
    -d "{\"ip\":\"${ircHost}\",\"username\":\"${IRC_USER}\",\"password\":\"${IRC_PASS}\"}" \
    >/dev/null
done <<< "${IRC_HOSTS}"

if [[ -z "${firstIrc}" ]]; then
  echo "ERROR: no IRC hosts generated" >&2
  exit 1
fi

trim() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  echo -n "${s}"
}

detect_port_map_from_process() {
  ps -eo args | awk '
    /racdevice/ && /-portMap/ {
      for (i = 1; i <= NF; i++) {
        if ($i == "-portMap" && i < NF) {
          print $(i+1)
          exit
        }
        if (index($i, "-portMap=") == 1) {
          val = $i
          sub(/^-portMap=/, "", val)
          print val
          exit
        }
      }
    }
  '
}

expand_port_spec() {
  local spec="$(trim "$1")"
  if [[ -z "${spec}" ]]; then
    echo "ERROR: empty port spec" >&2
    exit 1
  fi
  if [[ "${spec}" == *-* ]]; then
    local start="${spec%-*}"
    local end="${spec#*-}"
    if ! [[ "${start}" =~ ^[0-9]+$ && "${end}" =~ ^[0-9]+$ ]]; then
      echo "ERROR: invalid port range '${spec}'" >&2
      exit 1
    fi
    if (( start > end )); then
      echo "ERROR: port range start must be <= end (${spec})" >&2
      exit 1
    fi
    for ((p=start; p<=end; p++)); do
      echo "${p}"
    done
  else
    if ! [[ "${spec}" =~ ^[0-9]+$ ]]; then
      echo "ERROR: invalid port '${spec}'" >&2
      exit 1
    fi
    echo "${spec}"
  fi
}

SERVICE_ITEMS=""

if [[ -z "${DEVICE_PORT_MAP}" && "${AUTO_PORT_MAP_FROM_PROCESS}" == "true" ]]; then
  DEVICE_PORT_MAP="$(detect_port_map_from_process || true)"
  DEVICE_PORT_MAP="$(trim "${DEVICE_PORT_MAP}")"
  if [[ -n "${DEVICE_PORT_MAP}" ]]; then
    echo "Detected DEVICE_PORT_MAP from running racdevice process: ${DEVICE_PORT_MAP}"
  fi
fi

if [[ "${RANDOMIZE_RACDEVICES}" == "true" && -z "${DEVICE_PORT_MAP}" ]]; then
  echo "ERROR: RANDOMIZE_RACDEVICES=true requires mixed DEVICE_PORT_MAP (or detectable racdevice -portMap)." >&2
  echo "Example: DEVICE_PORT_MAP='idrac:60001-60010,nvlink:60011-60020,ami:60021-60025'" >&2
  exit 1
fi

build_service_items_from_hosts() {
  local hosts="$1"
  local out=""
  while read -r host; do
    [[ -z "${host}" ]] && continue
    out+="${RACDEVICE_SYSTEMTYPEDESC}|${host}"$'\n'
  done <<< "${hosts}"
  echo -n "${out}"
}

build_service_items_from_port_map() {
  local map="$1"
  local out=""
  IFS=',' read -r -a entries <<< "${map}"
  for entry in "${entries[@]}"; do
    entry="$(trim "${entry}")"
    [[ -z "${entry}" ]] && continue
    if [[ "${entry}" != *:* ]]; then
      echo "ERROR: DEVICE_PORT_MAP entry '${entry}' missing ':'" >&2
      exit 1
    fi
    local systemType="$(trim "${entry%%:*}")"
    local portSpec="$(trim "${entry#*:}")"
    if [[ -z "${systemType}" || -z "${portSpec}" ]]; then
      echo "ERROR: invalid DEVICE_PORT_MAP entry '${entry}'" >&2
      exit 1
    fi
    while read -r port; do
      [[ -z "${port}" ]] && continue
      out+="${systemType}|${RACDEVICE_IP}:${port}"$'\n'
    done < <(expand_port_spec "${portSpec}")
  done
  echo -n "${out}"
}

if [[ -n "${DEVICE_PORT_MAP}" ]]; then
  SERVICE_ITEMS="$(build_service_items_from_port_map "${DEVICE_PORT_MAP}")"
else
  if [[ -z "${RACDEVICE_HOSTS}" ]]; then
    if [[ -z "${RACDEVICE_PORT_END}" ]]; then
      RACDEVICE_PORT_END=$((RACDEVICE_PORT_START + RACDEVICE_PORT_COUNT - 1))
    fi
    RACDEVICE_HOSTS=""
    for ((p=RACDEVICE_PORT_START; p<=RACDEVICE_PORT_END; p++)); do
      RACDEVICE_HOSTS+="${RACDEVICE_IP}:${p}"$'\n'
    done
  fi
  SERVICE_ITEMS="$(build_service_items_from_hosts "${RACDEVICE_HOSTS}")"
fi

if [[ -z "${SERVICE_ITEMS}" ]]; then
  echo "ERROR: no service items generated" >&2
  exit 1
fi

post_service_item() {
  local ircHost="$1"
  local systemType="$2"
  local host="$3"
  local ircPath
  ircPath="$(urlencode_path_segment "${ircHost}")"

  local payload="{\"ip\":\"${host}\",\"username\":\"${RACDEVICE_USER}\",\"password\":\"${RACDEVICE_PASS}\",\"systemtypedesc\":\"${systemType}\",\"isforcefulshutdown\":${RACDEVICE_FORCEFUL}"
  if [[ "${RACDEVICE_FORCEFUL}" == "false" ]]; then
    payload+=",\"timeout\":${RACDEVICE_TIMEOUT}"
  fi
  payload+="}"

  $CURL -X POST "${BASE_URL}/api/v1/irc/${ircPath}/racdevice" \
    -H "Content-Type: application/json" \
    -u "${ADMIN_USER}:${ADMIN_PASS}" \
    -d "${payload}" \
    >/dev/null
}

add_racdevices_to_irc() {
  local ircHost="$1"
  while IFS='|' read -r systemType host; do
    [[ -z "${systemType}" || -z "${host}" ]] && continue
    post_service_item "${ircHost}" "${systemType}" "${host}"
  done <<< "${SERVICE_ITEMS}"
}

add_single_service_item_to_irc() {
  local ircHost="$1"
  local line="$2"
  local systemType host
  IFS='|' read -r systemType host <<< "${line}"
  [[ -z "${systemType}" || -z "${host}" ]] && return
  post_service_item "${ircHost}" "${systemType}" "${host}"
}

if [[ "${ASSOCIATE_EACH_IRC}" == "true" ]]; then
  while read -r ircHost; do
    [[ -z "${ircHost}" ]] && continue
    add_racdevices_to_irc "${ircHost}"
  done <<< "${IRC_HOSTS}"
else
  if [[ "${RANDOMIZE_RACDEVICES}" == "true" ]]; then
    mapfile -t irc_list < <(printf '%s' "${IRC_HOSTS}" | awk 'NF')
    mapfile -t service_items_list < <(printf '%s' "${SERVICE_ITEMS}" | awk 'NF')

    if (( ${#irc_list[@]} == 0 )); then
      echo "ERROR: no IRC hosts generated" >&2
      exit 1
    fi

    for line in "${service_items_list[@]}"; do
      ircHost="${irc_list[$(( RANDOM % ${#irc_list[@]} ))]}"
      add_single_service_item_to_irc "${ircHost}" "${line}"
    done
  elif [[ "${DISTRIBUTE_RACDEVICES}" == "true" ]]; then
    mapfile -t irc_list < <(printf '%s' "${IRC_HOSTS}" | awk 'NF')
    mapfile -t service_items_list < <(printf '%s' "${SERVICE_ITEMS}" | awk 'NF')

    if (( ${#irc_list[@]} == 0 )); then
      echo "ERROR: no IRC hosts generated" >&2
      exit 1
    fi

    idx=0
    for line in "${service_items_list[@]}"; do
      ircHost="${irc_list[$(( idx % ${#irc_list[@]} ))]}"
      add_single_service_item_to_irc "${ircHost}" "${line}"
      idx=$((idx + 1))
    done
  else
    add_racdevices_to_irc "${firstIrc}"
  fi
fi

echo "OK"
