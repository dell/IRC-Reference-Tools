# AGENTS.md — IRC-Reference-Tools

Liquid cooling leak detection and rack device shutdown system.
See [README.md](README.md) for full project overview.

## Quick Reference

| Action | Command |
|---|---|
| Start system | `bash docker-compose-files/compose.sh up` |
| Start (rebuild) | `bash docker-compose-files/compose.sh up --build` |
| Stop system | `bash docker-compose-files/compose.sh down` |
| View logs | `bash docker-compose-files/compose.sh logs --tail=50` |
| Container status | `bash docker-compose-files/compose.sh ps` |
| Unit tests | `go test ./... -race` |
| Integration tests | `go test -tags=integration ./tests/integration -v` |
| Test web UI | `curl -k https://localhost` |

## Architecture

Six Docker containers communicate via STOMP protocol over ActiveMQ:

| Container | Role | Entry point |
|---|---|---|
| **configui** | Web UI + REST API (HTTPS :443, Gin + React) | `cmd/configui/` |
| **dbdiscauth** | Device onboarding, credential storage, MySQL persistence | `cmd/dbdiscauth/` |
| **redfishread** | Redfish SSE event monitoring from IRC devices | `cmd/redfishread/` |
| **action** | Shutdown execution on rack devices when leak detected | `cmd/action/` |
| **activemq** | STOMP message bus (port 61613) | third-party image |
| **mysqldb** | MySQL database (port 3306) | third-party image |

Docker setup: [`docker-compose-files/docker-compose.yml`](docker-compose-files/docker-compose.yml)

## Project Layout

```
cmd/
  action/          # Shutdown action service
  configui/        # Web UI backend (Gin HTTP server)
  dbdiscauth/      # Device + auth persistence service
  redfishread/     # Redfish SSE listener
  frontend/        # React frontend (Material-UI)
internal/
  redfish/         # Configurable Redfish client
  systemtype/      # YAML config loader + registry
  httpclient/      # HTTP helpers (GET/POST/PATCH/DELETE)
  logging/         # Structured logging (slog)
  loginauth/       # JWT auth + middleware
  valve/           # Valve status via Redfish
  watchdog/        # Watchdog timer via Redfish
  tcpconnect/      # TCP connectivity check
configs/systemtypes/
  default/         # Built-in types: idrac, ami, cdu_lantern, nvlink
  custom/          # User-defined types (add YAML here)
  internal/        # Parent types (irc)
simulator/         # IRC + rack device simulators for testing
```

## Go Module

- Module: `github.com/dell/IRC-Reference-Tools`
- Go version: 1.24 (see `go.mod`)
- Key dependency: `github.com/dell/iDRAC-Telemetry-Reference-Tools` (pkg/auth, pkg/databus, pkg/messagebus)
- Web framework: `github.com/gin-gonic/gin`

## System Type Configuration

YAML files in `configs/systemtypes/` define per-device-type behavior (power monitoring, shutdown actions, SSE filters, system info endpoints). See [Custom System Types](configs/systemtypes/custom/README.md) for the full field reference.

## REST API

Base URL: `https://localhost/api/v1` — supports JWT (`Authorization: Bearer <token>`) and Basic auth.
Default credentials: `admin` / `admin`.
See [docs/API.md](docs/API.md) for all endpoints and curl examples.

Key endpoints:
- `POST /api/v1/GetToken` — authenticate
- `GET /api/v1/irc` — list IRCs
- `POST /api/v1/irc` — add IRC
- `GET /api/v1/irc/:ip/racdevice` — list rack devices for IRC
- `POST /api/v1/irc/:ip/racdevice` — add rack device (`systemtypedesc` must match `name` in a YAML config)
- `DELETE /api/v1/irc` — remove IRCs (body: `{"ips": ["ip1", "ip2"]}`)
- `DELETE /api/v1/irc/:ip/racdevice` — remove rack devices

## Leak Detection Flow

Leak message ID: `Dell.0.1.LCPO009`. When detected, all associated rack devices are shut down concurrently (force by default, graceful optional with timeout escalation). See [docs/LEAK-DETECTION-WORKFLOW.md](docs/LEAK-DETECTION-WORKFLOW.md).

## Environment Variables

Set via `docker-compose-files/compose.sh` (auto-generated `.env`):

| Variable | Purpose |
|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn` (default), `error` |
| `MYSQL_PASSWORD` | MySQL user password (auto-generated) |
| `MYSQL_ROOT_PASSWORD` | MySQL root password (auto-generated) |
| `CONFIGUI_JWT_SECRET` | JWT signing secret (auto-generated) |
| `MESSAGEBUS_HOST` | ActiveMQ host (default: `activemq`) |
| `MESSAGEBUS_PORT` | STOMP port (default: `61613`) |

## CI / GitHub Actions

- **ci.yml** — runs `go test ./... -race` on every PR and push to main
- **integration.yml** — full docker-compose stack + `go test -tags=integration` (triggered by `run-integration` label or workflow_dispatch)

See [`.github/workflows/`](.github/workflows/).

## Simulator

Local testing without real hardware. See [simulator/README.md](simulator/README.md).

### TLS certificates

Generate a self-signed cert+key pair before starting simulators:

```bash
CERT=~/localhost.crt
KEY=~/localhost.key
openssl req -x509 -out "$CERT" -keyout "$KEY" \
  -newkey rsa:2048 -nodes -sha256 -days 365 \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1" \
  -addext "keyUsage=digitalSignature" \
  -addext "extendedKeyUsage=serverAuth"
```

### Running simulators

```bash
cd simulator/irc && go run . -port-range 8443-8450 -basic-user <user> -basic-pass <pass> -tls-cert ~/localhost.crt -tls-key ~/localhost.key
cd simulator/racdevice && go run . -portMap "idrac:60001-60010" -tls -tlsCert ~/localhost.crt -tlsKey ~/localhost.key -username <user> -password <pass>
bash simulator/add_irc_racdevice.sh IRC 8443-8450 RACDEVICE 60001-60035
```

Supported rack device types: `idrac`, `nvlink`, `ami`, `cdu_lantern`, `nvbmc`.

**IMPORTANT — Simulator IP**: When adding simulated devices to ConfigUI, use the host machine's real IP (e.g., `hostname -I | awk '{print $1}'`), **NOT** `localhost`. Docker containers cannot reach simulators via `localhost` since that resolves to the container itself. Set `IRC_IP` and `RACDEVICE_IP` to the host IP.

**Before restarting simulators**, kill any existing instances to avoid "address already in use" errors:
```bash
pkill -f "simulator/irc"; pkill -f "simulator/racdevice"; sleep 2
```

## Key Internal Patterns

- All Docker operations go through `docker-compose-files/compose.sh`, not raw `docker compose`.
- Containers run as `irc:irc` (UID/GID 3012); configs mount at `/configs/systemtypes/` inside containers.
- Docker network: `irc-reference-tools_host-bridge-net`; DB volume: `irc-reference-tools_mysqldb-volume`.
- Go binaries built with `CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s"` (static, stripped).
- Power-off detection: `redfish-sse` (event-driven) or `tcp-ping` (polls ports 22/80/443) — configured per system type YAML.

## Documentation Index

| Doc | Contents |
|---|---|
| [README.md](README.md) | Project overview, quick start, screenshots |
| [docs/SETUP.md](docs/SETUP.md) | Prerequisites, installation, deployment, custom certs |
| [docs/API.md](docs/API.md) | REST API endpoints, auth, curl examples |
| [docs/LEAK-DETECTION-WORKFLOW.md](docs/LEAK-DETECTION-WORKFLOW.md) | End-to-end leak detection and shutdown flow |
| [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Common issues, diagnostics, log collection |
| [configs/systemtypes/custom/README.md](configs/systemtypes/custom/README.md) | YAML config format for custom device types |
| [simulator/README.md](simulator/README.md) | Simulator setup and usage |
| [cmd/frontend/README.md](cmd/frontend/README.md) | React frontend dev notes |
