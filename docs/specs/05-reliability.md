# 05 — Reliability and self-healing

Status: Draft v0.1 · Related: REL-*

**Goal:** a correctly installed device on working hardware never needs a person on site.

## Layers of recovery

Each layer handles what the one above it can't:

| # | Failure | Detection | Recovery |
|---|---|---|---|
| 1 | Player shows a blank or frozen screen, or content fails | Player heartbeat + "frame rendered" signal to the agent | Reload the window → restart the player |
| 2 | Player or compositor process crashes | systemd | `Restart=always` with back-off |
| 3 | Repeated crash loop | systemd `StartLimit*` | Restart the whole graphics session → reboot |
| 4 | Kernel or system hang | Hardware watchdog (`RuntimeWatchdogSec`) | Hard reset |
| 5 | New OS version doesn't boot | Bootloader boot counter | Fall back to the previous slot |
| 6 | New OS version boots but is broken | `castmill-health` fails | `mark-bad` → reboot into the previous slot |
| 7 | Corrupt rootfs block | dm-verity | Reboot; on repeat, switch slot |
| 8 | Corrupt or full `data` partition | Mount failure / free-space check | Fsck → recreate cache → (last resort) re-provision state while keeping identity |

## Health check (`castmill-health`)

Runs after every boot into an uncommitted slot, and periodically afterwards (report-only).

A slot is committed when all of the following pass within **N minutes** (default 10):

- [ ] system reached `multi-user.target` with no failed critical units
- [ ] `data` mounted and writable
- [ ] the compositor is running and at least one output is enabled
- [ ] the player reports "frame rendered" on every connected output
- [ ] the agent reached the Castmill backend **or** the network is confirmed down for reasons outside the device (no link / no DHCP). A network outage must not cause a rollback.

On failure: record the reason in `/data/log`, run `rauc status mark-bad`, reboot. After rollback, the agent reports the reason to the backend, which counts it towards the rollout halt threshold.

## Power loss and storage

- Rootfs read-only. All writes go to `data` (ext4 with journaling, `commit=` tuned).
- Updates write the **inactive** slot only. Losing power during installation leaves the old slot untouched and bootable.
- journald: volatile by default, plus a small persistent ring buffer on `data` (for example 16 MB) for post-mortem debugging.
- No swap on flash. **zram** for compressed swap in RAM.
- Player content cache has a size limit and an eviction policy, and monitors free space.
- Storage health monitoring (eMMC/SSD wear indicators where available) is reported to the backend.
- **Recommendation for deployments:** eMMC/NVMe/SSD over SD cards; industrial-grade SD cards when SD is unavoidable.

## Time

TLS, schedules and maintenance windows all need correct time, and many devices (including the Pi) have no battery-backed clock.

- `systemd-timesyncd` with several NTP servers plus a configurable customer NTP server.
- Fallback: get the time from the HTTPS `Date` header of the Castmill backend when NTP (UDP 123) is blocked.
- The last known time is saved on shutdown and periodically, so the clock never starts before the build date.
- Until time is trusted, schedules use cached content and maintenance-window activation is postponed.

## Network

- NetworkManager with priorities: Ethernet > Wi-Fi > (later) LTE.
- Automatic reconnect; Wi-Fi roaming between configured networks.
- Connectivity check against Castmill; captive portals are detected and reported (not handled automatically).
- The player keeps playing cached content while offline.

## Display

- Hotplug: when a display is disconnected and reconnected, the compositor restores the configured mode and rotation, and the player restores the window on that output.
- When the EDID is missing or wrong (common with HDMI extenders and matrices), a forced mode per output can be set in configuration.
- Display power schedules through CEC or DDC/CI. The device itself never sleeps.

## Thermal

- Report temperature and throttling state (Pi: `vcgencmd get_throttled` equivalent; x86: thermal zones).
- Alert in Castmill when a device throttles for long periods (bad ventilation is a common field problem).
