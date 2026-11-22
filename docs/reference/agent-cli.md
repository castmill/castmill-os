# castmill-agent

`castmill-agent` is the Castmill OS device agent: a single Go binary with several roles. Most of them are started by systemd; `status` and `update` are useful by hand on development images.

```text
castmill-agent <command> [options]
```

## Commands

| Command | Run by | Purpose |
|---|---|---|
| `run` | `castmill-agent.service` (user `agent`) | The agent daemon: the native side of the `castmillOS` bridge, player supervision, updates |
| `health` | `castmill-health.service` (root) | Post-boot health check: commits a trial slot or rolls it back |
| `health --report-only` | `castmill-health-report.timer` | Periodic check that never changes slots or reboots |
| `provision` | `castmill-provision.service` (root) | Applies [`castmill.conf`](../setup/castmill-conf.md) |
| `update URL VERSION [--now]` | you | Requests an OS update from a RAUC bundle URL. See [Updates](../operate/updates.md). |
| `session-config weston\|disabled` | the display session | Writes the compositor configuration |
| `status` | you | Prints the agent state as JSON |
| `version` | you | Prints the agent version |

## Options

| Option | Effect |
|---|---|
| `--config FILE` | Agent configuration (default `/etc/castmill/agent.toml`) |
| `-v` | Debug logging |
| `--now` | `update`: activate as soon as it's installed, ignoring the maintenance window |
| `--report-only` | `health`: report only, never mark slots or reboot |

## `status`

```sh
castmill-agent status
```

Prints the runtime state and the update state. The most useful fields:

| Field | Meaning |
|---|---|
| `agent_version`, `device_name` | Version and device name |
| `backend_url`, `backend_ok`, `backend_error` | The Castmill server, when it last answered, and the last error |
| `time_trusted` | Whether the clock has been set from NTP or HTTPS |
| `outputs` | Each output: connected, player attached, time of the last rendered frame |
| `update.state` | See [update states](../operate/updates.md#update-states) |
| `update.version`, `update.from_version`, `update.last_error` | The update and its outcome |

## Files

| Path | Contents |
|---|---|
| `/etc/castmill/agent.toml` | Agent configuration, part of the image |
| `/run/castmill-agent/player.sock` | Socket between the kiosk shells and the agent ([protocol](../castmill-integration.md#agent-socket)) |
| `/run/castmill-agent/state.json` | Runtime state (memory only) |
| `/data/castmill/state/` | Persistent state: device ID, update state, requested update |
| `/data/castmill/config/` | Provisioning results and per-device configuration |

The source is in [`agent/`](https://github.com/castmill/castmill-os/tree/main/agent).
