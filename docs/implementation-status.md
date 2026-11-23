# Implementation status (v0.1)

What exists for each requirement in [spec 01](specs/01-vision-and-requirements.md), and how far it has been verified.

**Verification levels:** **tested** = covered by automated tests that run in `checks.yml` (`go test`, `npm run check`) · **parsed** = the Yocto metadata parses and its dependency graph resolves (`bitbake -g`), but no image has been built or booted yet · **–** = not started.

> No image has been built or booted yet. The first green `build.yml` run and a boot in the development VM are the next milestones (roadmap Phase 0). Expect fixes in the Yocto layer when that happens.

## Security

| ID | Status | Where | Verified |
|---|---|---|---|
| SEC-001 | Read-only ext4 rootfs (`read-only-rootfs`). **dm-verity not yet** (kernel options enabled; needs an initramfs or UKI that sets up verity). The spec asks for erofs/squashfs; ext4 is used until verity lands. | `castmill-os-image.bb` | parsed |
| SEC-002 | RAUC verity bundles signed with X.509. Dev keys outside the repo; the device keyring comes from `CASTMILL_KEYS_DIR`. | `castmill-os-bundle.bb`, `rauc-conf.bbappend`, `scripts/gen-dev-keys.sh` | parsed |
| SEC-003 | – (Phase 2: Secure Boot + UKI; Pi signed boot) | | |
| SEC-004 | nftables: drop all inbound except established/related, ICMP errors, IPv6 ND, DHCPv6. SSH only on dev images. | `castmill.nft`, `castmill-dev` | parsed |
| SEC-005 | **Revised by [ADR-0008](adr/0008-reuse-castmill-device-stack.md):** devices use Castmill's device token from pincode pairing (no per-device certificates or mTLS). Device ID: random, kept on `/data`, new after a factory reset. | Castmill device app, `agent/internal/agent` | tested (ID) |
| SEC-006 | – (key on `/data`, 0600, agent user; TPM in Phase 2) | | |
| SEC-007 | – (Phase 2) | | |
| SEC-008 | Agent downloads and installs updates automatically; activation per maintenance window or security policy. | `agent/internal/updater` | tested |
| SEC-009 | SPDX SBOM (`create-spdx`, OE-core default) uploaded by `build.yml`. **CVE report not yet**: Wrynose replaced `cve-check` with `sbom-cve-check`, still to be wired into CI. | `castmill.conf` | parsed |
| SEC-010 | `player` and `agent` users with fixed UIDs; systemd sandboxing on every Castmill service; polkit limits the agent to rebooting and restarting the session/players; D-Bus policy limits the agent to RAUC's installer interface. Electron: `contextIsolation`, `sandbox`, no Node.js in renderers, navigation allowlist, permissions denied by default. | `castmill-base`, unit files, `player/app` | parsed / tested (player policy) |

## Reliability

| ID | Status | Where | Verified |
|---|---|---|---|
| REL-001 | GRUB boot counting (3 attempts) + `castmill-health` mark-bad → fallback; a rolled-back version is not retried. | `castmill-grub.cfg.in`, `agent/internal/health`, `agent/internal/updater` | tested (logic) / parsed (boot) |
| REL-002 | Trial boot detection; commit only when units, `/data`, outputs, frames on every connected output and backend (or external network outage) all pass within 10 min. A network outage never causes a rollback. | `agent/internal/health` | tested |
| REL-003 | `RuntimeWatchdogSec=30s`; watchdog drivers in the kernel config; the VM gets an emulated `i6300esb`. Agent (`WatchdogSec=60`) and session (Weston `systemd-notify`) are watched by systemd. | `system.conf`, `castmill.cfg`, `run-vm.sh` | parsed |
| REL-004 | Frame heartbeat → reload → restart player; `Restart=always`; player crash loop → session restart; session crash loop → reboot. | `agent/internal/agent` (frame watchdog), unit files | partly tested |
| REL-005 | Updates write only the inactive slot; `activate-installed=false` so a power cut can't activate an update early; atomic writes (fsync + rename) for all agent state. | `system.conf` (RAUC), `config.WriteFileAtomic` | tested (logic) |
| REL-006 | Volatile logs + 16 MB persistent journal on `/data`; zram swap; `noatime`, `commit=30`. | `castmill-base` | parsed |
| REL-007 | A `factory-reset` file on the boot partition: `/data` is recreated at boot. Factory reset from Castmill needs Castmill support (the agent's reset flag is ready). | `castmill-data-setup`, `agent/internal/system` | parsed |
| REL-008 | Agent reconnects with back-off; NetworkManager retries forever; clock from the HTTPS `Date` header when NTP is blocked; activation postponed until time is trusted; DRM hotplug starts/stops players. | agent, `castmill-outputs`, udev rule | partly tested |

## Updates

| ID | Status | Where | Verified |
|---|---|---|---|
| UPD-001 | RAUC A/B (ext4 slots) with the GRUB backend on UEFI targets. **Raspberry Pi 5: not yet** (tryboot vs U-Boot is a Phase 0 decision; the Pi image has the same partitions but no RAUC). | `rauc-conf.bbappend`, wks files | parsed |
| UPD-002 | Installation streams into the inactive slot in the background; the player keeps running. | updater | tested (logic) |
| UPD-003 | Maintenance windows (per day, crossing midnight, device time zone), "apply now", 24 h security policy. | `agent/internal/maintenance` | tested |
| UPD-004 / 005 | Planned in the Castmill backend (ADR-0005, ADR-0008); until then `castmill-agent update` on the device. | [Castmill integration](castmill-integration.md#os-updates) | – |
| UPD-006 | HTTP streaming with mTLS and adaptive updates (`block-hash-index`). | bundle recipe, agent | parsed |

## Display and media

| ID | Status | Where | Verified |
|---|---|---|---|
| MED-001 | Weston kiosk-shell on KMS/DRM, Electron as a native Wayland client. | `castmill-session`, `weston_%.bbappend` | parsed |
| MED-002 / 003 / 004 | Platform Chromium flags (VA-API on Intel). **The Intel media driver (iHD) isn't in the image yet** (meta-intel decision, Phase 0). Benchmark matrix pending. | `castmill-player` recipe | – |
| MED-005 | One player process per output, pinned by app ID; per-output mode, rotation and scale from Castmill. | `agent/internal/session`, `castmill-player@.service` | tested (config generation) |
| MED-006 | Castmill's device schedule: the device app blanks the screen; the agent puts the display in standby over HDMI-CEC or DDC/CI and wakes it. Saved on `/data`, applied offline, accepted by the health check while displays are in standby. | `agent/internal/display`, `agent/internal/agent/display.go`, `player/app/bridge.js` | tested (schedule, encoding, VM without CEC/DDC); CEC and DDC/CI not yet on real displays |
| MED-007 | Device info (OS, Electron, Chromium versions) goes to Castmill through the bridge; per-codec decode capabilities are not reported yet. | `player/app/bridge.js` | tested |

## Device management

| ID | Status | Where | Verified |
|---|---|---|---|
| MGT-001 | Through Castmill's device app and dashboard: restart app, reboot device (bridge → agent), refresh, clear cache, telemetry. Screenshots and log upload don't exist in Castmill yet. | `player/app/bridge.js`, agent | tested (bridge) |
| MGT-002 | – (Phase 2) | | |
| MGT-003 | The agent is a separate service; the frame watchdog reloads and restarts a hung player. Remote access while the player is down needs Castmill support. | agent | tested |
| MGT-004 | CPU usage and load, memory, `/data` usage, temperatures, network type, IP and Wi-Fi signal, uptime, in Castmill's `TelemetryData` format. Pi throttling, storage wear and dropped frames aren't in that format yet. | `agent/internal/telemetry` | tested |

## Installation and provisioning

| ID | Status | Where | Verified |
|---|---|---|---|
| PRV-001 | – (x86 USB installer) | | |
| PRV-002 | Pi image as `wic.zst` + `bmap` (flashable with bmaptool; Imager JSON later). | `kas/rpi5.yml` | parsed |
| PRV-003 | Castmill's pincode and QR code on screen (Castmill device app). Zero-touch bulk enrollment with an organization token needs Castmill support. | Castmill device app | – |
| PRV-004 | – | | |
| PRV-005 | `castmill.conf`: Wi-Fi, proxy (+ CA), NTP, time zone; moved into `/data` and deleted from the boot partition. USB sticks need an automounter (not yet). Proxy CA isn't imported into Chromium yet. | `agent/internal/provision` | tested |

## Development

| ID | Status | Verified |
|---|---|---|
| DEV-001 / 002 | `scripts/run-vm.sh` (2 outputs, watchdog); pairing with castmill.net or a local Castmill backend; `castmill-agent update` for update and rollback tests. | not booted yet |
| DEV-003 | `build.yml` builds all targets and uploads images. | not run yet |
| DEV-004 | `kas/dev.yml` → `castmill-dev` (SSH firewall rule), backend override allowed only there, red "DEVELOPMENT IMAGE" badge on screen. | parsed |

## Licensing

| ID | Status | Where | Verified |
|---|---|---|---|
| LIC-001 | `REUSE.toml` + `LICENSES/`; `reuse lint` passes. Not yet in CI (`checks.yml` job to add). | `REUSE.toml` | tested (locally) |
| LIC-002 | – (`INCOMPATIBLE_LICENSE` not set yet; impact to check in Phase 0) | | |
| LIC-003 | – (release pipeline) | | |

## Known gaps and decisions to make

- **License checksums for Go modules**: licensing is decided ([ADR-0007](adr/0007-licensing.md)); the agent recipe still needs per-module `LIC_FILES_CHKSUM` entries for its Go dependencies.
- **Backend**: devices load Castmill's device app from `https://castmill.net`. `CastmillOSMachine` (native integration) needs the Castmill change to be deployed; without it devices pair and play as plain web players. The backend CA isn't pinned (system roots).
- **Device location**: devices register without one (`getLocation` returns nothing). The plan is to set it in the Castmill dashboard, which already stores `loc_lat` and `loc_long`. GPS for moving screens comes later ([Castmill integration](castmill-integration.md#location)).
- **OS updates from Castmill**: release registry, desired version, rollouts and status in the Castmill backend (follow-up to ADR-0008).
- **Fallback screen** when the player can't start: the status page is part of the player process; a native fallback (spec 07) is still to do.
- **Electron app ID on Wayland**: verify `app.setDesktopName` → app ID with Weston kiosk-shell on the first boot (ADR-0004).
- **dm-verity**, erofs, Secure Boot, TPM, encrypted `/data`: Phase 1–2.
