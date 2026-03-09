# Simulator Quickstart

This folder hosts two lightweight Go simulators that mimic IRC services and rack devices so you can exercise ConfigUI end-to-end without real hardware.

## Contents

| Path | Description |
| --- | --- |
| `irc/` | Multi-port IRC (cooling loop) simulator that exposes minimal REST + SSE endpoints. |
| `racdevice/` | Redfish-compatible rack-device simulator supporting multiple device types (iDRAC, NVLink, AMI, CDU Lantern, NvBMC, etc.). |
| `seed_configui_irc_idracs.sh` | Helper script that registers simulator instances inside ConfigUI via its REST API. Always run this after the simulators are up. |
| `*.yaml` | Sample system-type definitions that mirror the real configs shipped with ConfigUI. |

## Prerequisites

- Go >= 1.18 (`go version`)
- Local TLS cert+key pair (defaults: `/home/<user>/localhost.crt` & `/home/<user>/localhost.key`)
- ConfigUI + auth stack running (e.g., via `docker-compose-files/compose.sh up`)
- Base URL & admin creds for ConfigUI (defaults below)

```text
BASE_URL=https://localhost
ADMIN_USER=admin
ADMIN_PASS=admin
SIM_USER=<USERNAME>
SIM_PASS=<PASSWORD>
```

> Tip: create a shell alias that exports these variables so the seeding script can reuse them.

## Running the IRC simulator

```bash
cd simulator/irc
# Default range 8443-8450, Basic Auth enabled
go run . -port-range 8443-8450 \
  -basic-user "${SIM_USER}" \
  -basic-pass "${SIM_PASS}"
```

- The process binds one HTTP server per port in the range.
- Use `Ctrl+C` to stop it when you are done.

## Running the rack-device simulator

```bash
cd simulator/racdevice
export PORT_MAP="idrac:60001-60010,nvlink:60011-60020,ami:60021-60025,cdu_lantern:60026-60030,nvbmc:60031-60035"

go run . -portMap "${PORT_MAP}" -tls -tlsCert /home/${USER}/localhost.crt -tlsKey /home/${USER}/localhost.key -username <username> -password <passowrd>
```

- `-portMap` accepts comma-separated `systemType:start-end` entries.
- Supported device types: `idrac`, `nvlink`, `ami`, `cdu_lantern`, `nvbmc`.
- The simulator enforces Basic Auth and emits SSE events that match the real configs (e.g., `RCDU005`/`RCDU011` for CDU Lantern).

## Seeding ConfigUI (always randomized)

Once both simulators are running, register them with ConfigUI from the root `simulator/` folder:

```bash
cd simulator
PORT_MAP="idrac:60001-60010,nvlink:60011-60020,ami:60021-60025,cdu_lantern:60026-60030,nvbmc:60031-60035" \
RANDOMIZE_RACDEVICES=true \
DEVICE_PORT_MAP="${PORT_MAP}" \
IRC_IP=100.71.133.166 \
RACDEVICE_IP=100.71.133.166 \
bash ./add_irc_racdevice.sh IRC 8443-8450 RACDEVICE 60001-60035

```

> `BASE_URL`, `ADMIN_USER`, and `ADMIN_PASS` default to `https://localhost`, `admin`, and `admin` inside the script. Only export them ahead of time if you need custom values.

What the script does:
1. Creates IRC services for every port in the given IRC range.
2. Builds service-item payloads from `DEVICE_PORT_MAP`.
3. Randomly assigns each rack device to one of the simulated IRCs (`RANDOMIZE_IDRACS=true` is required for mixed types).

## Verification

- **Simulators**: each process logs `listening on :<port>`. If you see `bind: address already in use`, stop the previous run and retry.
- **ConfigUI**: log in at `BASE_URL`, open the Services page, and confirm the seeded IRCs and rack devices appear with the expected system types.
- **Events**: watching `docker logs -f <configui_container>` should show `Got new event` lines when the rack-device simulator emits SSE updates.

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| `bind: address already in use` when starting a simulator | Another instance is still running. Stop it (Ctrl+C) or choose a different port range. |
| Seeding script exits without creating devices | Ensure `DEVICE_PORT_MAP` is non-empty and ConfigUI is reachable at `BASE_URL`. |
| CDU shutdown “Switch must be 0” error | The simulator expects body `{"Switch":"0"}` per the `cdu_lantern.yaml` config. |
| NvBMC never flips power state | NvBMC uses `redfish-get` polling; no SSE events are emitted. Confirm polling results instead. |

## Cleanup

- Stop each `go run` process with `Ctrl+C`.
- Optionally delete the seeded services inside ConfigUI (via UI or API) if you want a fresh run.
