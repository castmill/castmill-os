# Castmill OS kiosk shell

Electron app that hosts the Castmill player on a Castmill OS device: one
process per display output (`castmill-player@<output>.service`). It shows the
local status page (pairing code, errors) until the device is enrolled, then
the player URL from `castmill-agent`, and reports frames, GPU/decode
capabilities and playback stats to the agent. See
[spec 06](../docs/specs/06-display-and-media.md) and
[ADR-0004](../docs/adr/0004-sway-kiosk-compositor.md).

No npm dependencies: Electron itself is provided by the OS image
(`electron-bin` recipe).

## Checks

```sh
npm run check     # syntax + unit tests (navigation policy, agent link)
```

## Run it on a workstation

Run the agent against the mock backend (see [device API](../docs/device-api.md#mock-backend-development)),
then start the shell in a normal window with Electron 45:

```sh
npm install --no-save electron@45.4.1
env -u ELECTRON_RUN_AS_NODE \
  CASTMILL_WINDOWED=1 \
  CASTMILL_AGENT_SOCKET=<agent run_dir>/player.sock \
  CASTMILL_PLAYER_CONFIG=<agent data_dir>/config/agent/player.json \
  CASTMILL_USER_DATA=/tmp/castmill-player \
  npx electron . --castmill-output=Virtual-1
```

| Variable | Meaning |
|---|---|
| `CASTMILL_WINDOWED=1` | normal 960×540 window instead of a fullscreen kiosk window (development only) |
| `CASTMILL_AGENT_SOCKET` | agent socket (device: `/run/castmill-agent/player.sock`) |
| `CASTMILL_PLAYER_CONFIG` | `player.json` written by the agent (allowed origins) |
| `CASTMILL_USER_DATA` | Chromium profile directory; one subdirectory per output |

`ELECTRON_RUN_AS_NODE` must not be set (VS Code's terminal sets it), or
Electron runs as plain Node.js. On macOS, screenshots of a window hidden
behind other windows fail (Chromium stops drawing occluded windows); on a
device the kiosk window is always visible.
