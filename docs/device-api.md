# Device API (draft v1)

Status: Draft · Implemented by `castmill-agent` and by the development mock backend (`agent/cmd/castmill-mock-backend`). The Castmill backend implements the same API ([ADR-0005](adr/0005-update-rollout-in-castmill-backend.md), option A).

The wire types are defined in [`agent/internal/api/api.go`](../agent/internal/api/api.go); that file is the reference for field names.

## Transport and authentication

- Base path: `https://<backend>/api/os/v1`.
- TLS 1.2+. Devices pin the backend CA (`/etc/castmill/backend-ca.pem` in the image).
- **Enrollment** endpoints use server-authenticated TLS only (the device has no certificate yet).
- **Every other endpoint** requires mutual TLS with the device certificate (SEC-005). The certificate's subject CN is the device ID, and the backend rejects requests whose `{id}` doesn't match it.
- `401`/`403` means the device is unknown or revoked. The agent shows "revoked, factory reset needed" on screen and retries rarely (every 30 minutes).
- All requests are outbound from the device; no inbound ports (SEC-004). An HTTP(S) proxy from `castmill.conf` is honoured.
- The agent uses the HTTPS `Date` header as a clock source when NTP is blocked (spec 05).

## Enrollment (PRV-003)

```
Device                                           Backend
  │ POST /enrollments {csr, hardware, org_token?}  │
  │ ─────────────────────────────────────────────► │
  │ ◄── 201 {id, status: pending, pairing_code} ── │   (approved at once for a valid org_token)
  │   screen shows the pairing code                │
  │ GET /enrollments/{id}   (every poll_interval_s)│   operator enters the code in Castmill
  │ ◄── {status: approved, device_id, certificate} │
  │   stores key + certificate under /data         │
```

- The device generates its key pair locally (ECDSA P-256); only the CSR leaves the device.
- `hardware`: `machine`, `compatible` (RAUC compatible string), `model`, `serial`, `os_version`, `dev_image`.
- `org_token`, `group` and `name` come from `castmill.conf` for zero-touch bulk enrollment (spec 08). The device expands `name_template` (`{group}`, `{serial}`).
- A pending enrollment is persisted, so the pairing code survives agent restarts until it expires.
- Statuses: `pending`, `approved`, `rejected`, `expired`. On `rejected` or `expired` the device starts a new enrollment.

## Device endpoints (mTLS)

| Method | Path | Body / response |
|---|---|---|
| `GET` | `/devices/{id}/desired` | → `Desired {version, bundle_url, sha256, security, activate_policy, pinned}`. Empty `version`: nothing to do. |
| `GET` | `/devices/{id}/config` | → `DeviceConfig` (see below) |
| `POST` | `/devices/{id}/status` | ← `DeviceStatus` (versions, RAUC slots, update state, last health report, outputs, decode capabilities). `204`. |
| `POST` | `/devices/{id}/certificate` | ← `{csr}` → `{certificate}`. Renewal when a third of the validity is left. |
| `POST` | `/devices/{id}/uploads?kind=logs\|screenshot&command_id=…` | raw body (`application/gzip` or `image/png`) → `{id}` |
| `GET` | `/devices/{id}/ws` | WebSocket, see below |

Bundles (`bundle_url`) are fetched by RAUC with HTTP range requests, presenting the same device certificate. They can live on any object storage or CDN that supports range requests and mTLS, or behind a signed URL.

### `activate_policy`

| Value | Meaning |
|---|---|
| `window` (default) | Install now, reboot inside the device's maintenance window (UPD-003). |
| `now` | Operator forced "apply now". |
| `security` | Organisation security policy: with `security_policy: "24h"` the reboot happens within 24 hours of installation even outside the window. |

### `DeviceConfig`

```json
{
  "revision": 7,
  "timezone": "Europe/Madrid",
  "maintenance_windows": [{"days": ["mon", "tue"], "start": "03:00", "end": "05:00"}],
  "security_policy": "window",
  "player_url": "https://app.castmill.io/player?device={device_id}",
  "outputs": {"HDMI-A-1": {"mode": "1920x1080@60", "transform": "rotate-90", "scale": 1, "disabled": false}},
  "telemetry_interval_s": 300
}
```

Windows are in device-local time; an `end` earlier than `start` crosses midnight. With no windows, 03:00–05:00 applies. Output changes restart the display session (a few seconds of black screen).

## WebSocket

One JSON object per message. The agent keeps one connection open, pings every 30 s and reconnects with exponential back-off (1 s → 5 min, with jitter).

| `type` | Direction | Fields |
|---|---|---|
| `hello` | device → server | `status` |
| `command` | server → device | `id`, `command`, `args` |
| `result` | device → server | `id`, `ok`, `error`, `data` |
| `config` | server → device | `config` (applied immediately, stored under `/data`) |
| `desired` | server → device | `desired` (or empty: the device polls `/desired`) |
| `telemetry` | device → server | `telemetry` (CPU, memory, temperatures, Pi throttling, `/data` usage, eMMC wear, per-output dropped frames) |

### Commands (MGT-001)

| `command` | `args` | Result `data` |
|---|---|---|
| `reboot` | – | – (reboots 2 s after answering) |
| `restart_player` | `output` (optional) | `unit` |
| `reload_player` | `output` (optional) | `outputs` |
| `screenshot` | `output` (optional) | `uploads: {output: upload id}` |
| `collect_logs` | – | `upload`, `bytes` (journal of the current and previous boot, gzip) |
| `factory_reset` | `confirm` = device ID | – (wipes `/data` on the next boot, then reboots) |
| `check_update` | – | – |
| `apply_update_now` | – | – (activates an installed update now) |

## Update state machine (reported in `DeviceStatus.update.state`)

`idle` → `installing` (with `progress`) → `installed` → `activating` (reboot) → `running` (new version committed by the health check) → `idle`.
Failures: `failed` (installation error, retried with back-off) and `rolled_back` (with `last_error` from the health check; that version is not retried). See [spec 03](specs/03-updates.md) and [spec 05](specs/05-reliability.md).

## Mock backend (development)

```sh
cd agent
go run ./cmd/castmill-mock-backend --state ../.mock-backend --auto-approve --bundles ../build/tmp/deploy/images/genericarm64
```

- Device API on `https://127.0.0.1:8443`, reachable from the QEMU VM as `10.0.2.2:8443` (the certificate is valid for `localhost`, `127.0.0.1` and `10.0.2.2`). Use `--listen` to expose it to real devices on your network.
- Admin API on `http://127.0.0.1:8081`:

```sh
curl -s localhost:8081/devices | jq
curl -s -X POST localhost:8081/pair -d '{"code":"ABC-123"}'
curl -s -X POST localhost:8081/devices/dev-0002/command -d '{"command":"screenshot"}'
curl -s -X POST localhost:8081/devices/dev-0002/desired \
  -d '{"version":"2026.11.1-dev","bundle_url":"https://10.0.2.2:8443/bundles/castmill-os-bundle-genericarm64.raucb","activate_policy":"now"}'
curl -s -X POST localhost:8081/devices/dev-0002/config -d '{"timezone":"Europe/Madrid","maintenance_windows":[{"start":"03:00","end":"05:00"}]}'
```

Point a development image at it with `castmill.conf` on the boot partition (the `[backend]` section is ignored on production images):

```toml
[backend]
url = "https://10.0.2.2:8443"
ca_cert_file = "server-ca.crt"   # copy from .mock-backend/server-ca.crt
```
