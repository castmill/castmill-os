# 02 — System architecture

Status: v0.3 · Describes what is built; items marked *planned* are not implemented yet ([implementation status](../implementation-status.md)).

## Overview

Castmill OS is built with the Yocto Project as a set of complete, signed system images, one per hardware family. Devices have no package manager. The whole operating system, including the Electron runtime and the Castmill player, is one versioned artifact that is tested as a unit and installed atomically.

```
┌───────────────────────── user space ─────────────────────────┐
│  Kiosk shell (Electron, user: player) → Castmill device app  │
│  castmill-agent  (user: agent; RAUC, NetworkManager, CEC)    │
│  castmill-videod (optional native video path, later)         │
├──────────────────────────────────────────────────────────────┤
│  Weston kiosk-shell (ADR-0004)                               │
│  Mesa (iris / v3d) · libva + Intel media driver (iHD)        │
├──────────────────────────────────────────────────────────────┤
│  systemd · NetworkManager · RAUC · nftables · timesyncd      │
│  journald (volatile + small persistent ring buffer)          │
├──────────────────────────────────────────────────────────────┤
│  Linux kernel (LTS; Raspberry Pi kernel fork on Pi)          │
└──────────────────────────────────────────────────────────────┘
```

## Build system

- **Yocto Project 6.0 "Wrynose" LTS** with one project layer, `meta-castmill`: distro configuration (`DISTRO = "castmill"`), images, the update bundle, the installer, agent, kiosk shell, session, boot configuration (GRUB on UEFI, firmware tryboot on the Pi) and kernel configuration fragments. Platform differences are machine overrides, not separate layers.
- Upstream layers: `bitbake`, `openembedded-core`, `meta-yocto` (generic machines only), `meta-openembedded`, `meta-rauc`, `meta-raspberrypi` (Pi 5), `meta-intel` (x86-64, Intel media driver only). Electron is the prebuilt release (`electron-bin` recipe). Layer versions are pinned per target (`kas/*.lock.yml`). *To evaluate:* `meta-security`.
- Builds are described with **kas** files so they are reproducible on any CI runner.
- See [10](10-build-release-testing.md).

## Image types

| Image | Contents | Use |
|---|---|---|
| `castmill-os-image` | Production rootfs | Installed into A/B slots |
| `castmill-os-bundle` | RAUC bundle of the rootfs | OTA updates |
| `castmill-os-installer` (x86) | USB stick that writes the disk image to the internal disk | Installing Intel PCs |
| Development image | `castmill-os-image` with SSH, root login and a "DEVELOPMENT IMAGE" badge (`kas/dev.yml`) | Development only |

### Build targets

| Target | Machine | Purpose |
|---|---|---|
| `x86-64` | `genericx86-64` | Intel/AMD mini PCs |
| `rpi5` | `raspberrypi5` | Raspberry Pi 5 / CM5 |
| `vm-arm64` | `genericarm64` (UEFI) | Development VM for Apple Silicon Macs (QEMU/UTM); see [development guide](../development.md) |

## Disk layout

| Partition | Size (initial) | Filesystem | Mount | Notes |
|---|---|---|---|---|
| `boot` / ESP | 256 MB (UEFI), 64 MB (Pi) | vfat | `/boot` | GRUB and its A/B state (UEFI); `autoboot.txt` (Pi); `castmill.conf`, `factory-reset` |
| `boot_a`, `boot_b` | 128 MB each | vfat | – | Pi only: kernel, device trees, `config.txt` per slot ([ADR-0009](../adr/0009-raspberry-pi-tryboot.md)) |
| `rootfs_a` | 1.5 GB | ext4, read-only (*planned:* erofs + dm-verity) | `/` | System, slot A |
| `rootfs_b` | 1.5 GB | ext4, read-only (*planned:* erofs + dm-verity) | `/` | System, slot B |
| `data` | rest | ext4 (*planned:* LUKS2 with the key in the TPM) | `/data` | Player profiles and cache, agent state, configuration, logs; grown on first boot |

Slot size leaves room for the image to grow. A slot is overwritten as a whole on update.

### Writable state

Everything writable lives under `/data`. Bind mounts and `systemd-tmpfiles` map it into place:

- `/data/castmill/player`: the kiosk shell's profiles (one per output: Castmill credentials, settings, cache)
- `/data/castmill/identity`, `/data/castmill/state`: the device ID, update state, device schedule
- `/data/castmill/config`: provisioning results, NetworkManager connections, per-device configuration
- `/data/log`: small persistent log ring buffer
- `/etc` is read-only, except for a minimal overlay limited to specific files (for example NetworkManager connections stored under `/data`).

**Factory reset** reformats `data`. On the next boot the device is a fresh, unpaired player with a new device ID.

## Boot flow

1. Firmware → bootloader with A/B slot selection and boot counting (GRUB boot counting on x86-64, the firmware's `tryboot` on Pi, [ADR-0009](../adr/0009-raspberry-pi-tryboot.md)).
2. Kernel, straight from the slot (no initramfs yet; *planned:* initramfs or UKI with dm-verity and the TPM unlock of `data`).
3. systemd → network, time sync, compositor, player, agent.
4. **Health check** (`castmill-health.service`, see [05](05-reliability.md)) → `rauc status mark-good` → boot counter reset.

## Runtime components

| Component | Responsibility | User |
|---|---|---|
| Weston kiosk-shell (`castmill-session`) | Wayland compositor; each kiosk shell's window fullscreen on its output; no shortcuts or launcher | `player` |
| Kiosk shell (`castmill-player@<output>`) | Electron, one process per output; loads Castmill's device app and offers it the `castmillOS` bridge; local status page | `player` |
| `castmill-agent` | Native side of the bridge, OS updates (RAUC D-Bus, maintenance windows, targets from Castmill), health, telemetry, provisioning, display power (CEC/DDC), on-site Wi-Fi | `agent` (limited capabilities) |
| `castmill-health` | Post-boot and periodic health checks; commits or rejects slots | `root` (oneshot) |
| `rauc` | Installs and verifies bundles; manages slots | `root` |
| NetworkManager | Ethernet, Wi-Fi, (later) LTE; profiles from `/data` | `root` |

The player and the agent are deliberately separate, so device management keeps working if the player hangs or crashes (MGT-003).

## Player delivery

The Electron runtime and the kiosk shell are part of the OS image; Castmill's device app is loaded from the Castmill server ([ADR-0008](../adr/0008-reuse-castmill-device-stack.md)). One image version = one tested combination. Electron's own auto-updater is disabled.

Trade-off: a player hotfix needs an OS release. Adaptive/streamed RAUC updates keep that cheap, and the release pipeline has to support fast patch releases. To revisit if the player's release pace requires it.

## Interfaces

- **Device ↔ Castmill:** Castmill's device app over its device channel (pairing, content, commands, telemetry, OS update targets and status); the agent only probes reachability and downloads bundles over HTTPS. See [Castmill integration](../castmill-integration.md).
- **Kiosk shell ↔ agent:** local Unix socket (`/run/castmill-agent/player.sock`: state, bridge calls, frame heartbeat, GPU report).
- **Agent ↔ system:** D-Bus to RAUC, systemd/logind and NetworkManager.
- **Session:** the agent writes the Weston configuration (outputs, rotation) and restarts the session; screenshots are *planned*.
