# Castmill integration

How a Castmill OS device works with the Castmill platform ([ADR-0008](adr/0008-reuse-castmill-device-stack.md)). Castmill OS is a Castmill platform like the Electron, Android and webOS players: Castmill's own device app does pairing, content, commands and telemetry, and Castmill OS provides the native side.

```
 Castmill backend (castmill.net)
   ▲  device app (HTTP)  ▲  /socket (Phoenix channels: pairing, commands, telemetry)
   │                     │
┌──┴─────────────────────┴──────────────────────────────┐
│ kiosk shell (Electron), one per output                 │
│   Castmill device app ── CastmillOSMachine             │
│                             │ window.castmillOS.call() │
│   preload ── main process (castmillOS bridge) ─────────┼─┐
└────────────────────────────────────────────────────────┘ │ Unix socket
┌──────────────────────────────────────────────────────────┴─┐
│ castmill-agent: machine ID, device info, telemetry, time    │
│ zone, reboot · A/B updates (RAUC) · health · provisioning   │
└─────────────────────────────────────────────────────────────┘
```

## Startup

1. `castmill-agent` writes the player URL, `<backend>/`, to `/data/castmill/config/agent/player.json` and sends it to every kiosk-shell process. The backend is `https://castmill.net` by default (`CASTMILL_BACKEND_URL`). Development images can override it with `[backend]` in `castmill.conf`.
2. Each kiosk-shell process (`castmill-player@<output>`) loads that URL: Castmill's device app.
3. The device app finds `window.castmillOS` and uses `CastmillOSMachine` instead of the browser machine.
4. The first time, it registers with Castmill: it shows a pincode and a QR code, and an operator claims the screen in the dashboard. Each output is a separate Castmill device.
5. From then on the device app connects to Castmill as usual. When the backend can't be reached, the kiosk shell shows its local status page and retries every 30 seconds.

## The castmillOS bridge (version 1)

`window.castmillOS` is exposed by the kiosk shell's preload script:

```ts
interface CastmillOSBridge {
  readonly version: number; // 1; later versions stay backwards compatible
  call(method: string, ...args: unknown[]): Promise<unknown>;
}
```

Every call is checked by the main process. It must come from the window's **main frame** on the **same origin as the player URL**. Plain `http` is accepted only for loopback, or for any host on development images. Other callers get an error, and preloads don't run in sub-frames, so iframes don't see the bridge at all.

| Method (`Machine`) | Arguments | Answered by | Result |
|---|---|---|---|
| `getSetting` | `key` | kiosk shell | value or `null`. Keys: `BASE_URL`, `TIMERS`, `TIMER_OFF`, `ORIENTATION` |
| `setSetting` | `key, value` | kiosk shell | stored in the output's profile on `/data` (string, ≤ 64 KiB). `TIMERS` is also sent to the agent ([Device schedule](#device-schedule)) |
| `storeCredentials` | `credentials` | kiosk shell | stored with mode 0600 in the output's profile |
| `getCredentials` | – | kiosk shell | string or `null` |
| `removeCredentials` | – | kiosk shell | – |
| `getMachineGUID` | – | agent | `<device id>-<output>`, stable until a factory reset |
| `getDeviceInfo` | – | agent + kiosk shell | `{appType: "Castmill OS", appVersion, os, hardware, environmentVersion, chromiumVersion, v8Version, nodeVersion}`; the device app adds `userAgent` |
| `getHardwareInfo` | – | agent + kiosk shell | `{cpu, memory, gpu, graphicsAcceleration: "hardware" \| "software", videoDecoding: ["H.264", …], display}`; waits up to 20 s for the GPU probe. Separate from `getDeviceInfo` because Castmill servers reject device info with unknown keys: only a device app that knows these fields asks for them (Castmill's `CastmillOSMachine` merges them into its device info) |
| `getTelemetry` | – | agent | Castmill `TelemetryData`: storage (the data partition), memory, CPU usage and load, temperatures, network, uptime |
| `getTimezone` | – | agent | IANA name, e.g. `Europe/Madrid` |
| `reboot` | – | agent | reboots one second later |
| `restart` | – | kiosk shell | restarts this output's player |
| `clearCache` | – | kiosk shell | clears Chromium's HTTP cache |

Not provided on purpose:
- `shutdown`: signage players stay on.
- `update`: the shell is part of the OS image.
- `updateFirmware`: OS updates come with the Castmill backend support (below).
- `setOrientation`: the device app rotates content with CSS.
- `getLocation`: see [Location](#location).

Castmill's `@castmill/device` derives the device capabilities from the methods `CastmillOSMachine` implements.

## Device schedule

Castmill's device schedule (on and off times per weekday) needs no Castmill change. `CastmillOSMachine` has no `setTimers`, so the device app uses its own timers, as on any platform without hardware timers:

1. The device app stores the schedule with `setSetting("TIMERS", …)` and blanks the screen and stops playback at the off times.
2. The kiosk shell sends every change of `TIMERS`, and the stored value whenever it connects, to the agent (`set_timers`).
3. The agent saves the schedule on `/data` and puts the display in standby at the off times and wakes it at the on times, over HDMI-CEC or DDC/CI ([Device schedule](operate/device-schedule.md)).

The agent evaluates the schedule itself, so a display whose player stopped (a TV that disconnects in standby) is still woken.

## Location

Castmill OS devices register **without a location**: `CastmillOSMachine.getLocation()` returns nothing. The browser's geolocation (`navigator.geolocation`, which Castmill's Electron player uses) doesn't work here. On Linux, Chromium resolves it through Google's location service, which needs an API key and sends data about nearby networks to Google.

Signage screens don't move, so the plan is to set the location **in the Castmill dashboard** instead:

- Castmill already stores a location per device (`loc_lat`, `loc_long`) and accepts it in device updates. What's missing is a location field on the device page; the map picker that widget settings use can be reused there.
- Location widgets (weather and others) should then default to the device's location. Today they default to London, United Kingdom, until a location is picked in the widget's settings.

This is Castmill work and needs no change to Castmill OS.

**Later: GPS.** Screens that move (buses, trains, taxis) need a live location. The agent can read it from a USB GNSS receiver or an LTE modem with GNSS (through `gpsd` or ModemManager) and report it with its telemetry. `getLocation` would then answer from it, and a live location would take precedence over the one set in the dashboard. Reporting a moving location would be opt-in per organisation. See the [roadmap](specs/11-roadmap.md#phase-3-expansion).

## Agent socket

Between the kiosk shell and `castmill-agent`, local only: `/run/castmill-agent/player.sock`, accessible to the `castmill` group. One JSON object per line ([`agent/internal/playerlink`](../agent/internal/playerlink/playerlink.go)).

| Direction | Message |
|---|---|
| shell → agent | `{"type":"hello","output":"HDMI-A-1"}` |
| shell → agent | `{"type":"frame","output":"HDMI-A-1"}`: frame-rendered heartbeat (health check, watchdog) |
| shell → agent | `{"type":"call","id":"7","method":"telemetry"}`. Methods: `machine_id`, `device_info`, `hardware_info` (`{cpu, memory}`), `telemetry`, `timezone`, `reboot`, `set_timers` (`args: {"timers": "<TIMERS setting>"}`), `gpu_report` (`args: {"rendering": "hardware", "renderer": "…", "video_decode": ["H.264", …], "features": {…}}`, sent once the GPU process is up) |
| agent → shell | `{"type":"reply","id":"7","ok":true,"data":{…}}` or `{"type":"reply","id":"7","error":"…"}` |
| agent → shell | `{"type":"state","state":{"player_url","dev_image","os_version","device_name","online","backend_error"}}` |
| agent → shell | `{"type":"reload"}`: no frames for 30 s (recovery layer 1, spec 05) |

## OS updates

The A/B update machinery is complete on the device: RAUC, maintenance windows, the health check and rollback. Until the Castmill backend can tell a device which OS version to run, an update is requested locally:

```sh
castmill-agent update https://example.com/castmill-os-bundle-genericarm64.raucb 0.2.0 [--now]
castmill-agent status
```

**Planned Castmill support** (follow-up to the `CastmillOSMachine` change):
- an OS release registry, with a manifest per release (version, compatible, bundle URL, security flag);
- a desired OS version per device or group, with channels and staged rollouts (spec 03);
- update status reported over the existing device channel, authenticated with the existing device token;
- `update_firmware` mapped to "check for an OS update now".

## Developing against a local Castmill

Run the Castmill backend from the [Castmill repository](https://github.com/castmill/castmill) (see its README). Point a development image at it with `castmill.conf` on the boot partition:

```toml
[backend]
url = "http://10.0.2.2:4000"   # the host as seen from the QEMU development VM
```

The kiosk shell can also run on a workstation against a local agent; see [`player/README.md`](../player/README.md).
