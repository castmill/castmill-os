# Castmill OS kiosk shell

Electron app that runs Castmill's device app on a Castmill OS device: one
process per display output (`castmill-player@<output>.service`). It loads the
device app from the Castmill backend (player URL from `castmill-agent`),
answers its native `Machine` calls through the `castmillOS` bridge, shows a
local status page while the backend can't be reached, and reports frames to
the agent. See [Castmill integration](../docs/castmill-integration.md),
[spec 06](../docs/specs/06-display-and-media.md) and
[ADR-0004](../docs/adr/0004-sway-kiosk-compositor.md).

No npm dependencies: Electron itself is provided by the OS image
(`electron-bin` recipe).

## Checks

```sh
npm run check     # syntax + unit tests (navigation policy, agent link)
```

## Run it on a workstation

Run `castmill-agent run --config <agent.toml>` with `[backend] url` pointing at
a Castmill backend (a local one on `http://localhost:4000` works), then start
the shell in a normal window with Electron 45:

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
