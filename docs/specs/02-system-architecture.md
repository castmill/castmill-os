# 02 — System architecture

Status: Draft v0.1

## Overview

Castmill OS is built with the Yocto Project as a set of complete, signed system images, one per hardware family. Devices have no package manager. The whole operating system, including the Electron runtime and the Castmill player, is one versioned artifact that is tested as a unit and installed atomically.

```
┌───────────────────────── user space ─────────────────────────┐
│  castmill-player (Electron, user: player)                    │
│  castmill-agent  (user: agent; talks to RAUC over D-Bus)     │
│  castmill-videod (optional native video path, later)         │
├──────────────────────────────────────────────────────────────┤
│  Kiosk compositor (Sway or Weston kiosk-shell, ADR-0004)     │
│  Mesa (iris / v3d) · libva (iHD) · GStreamer · PipeWire      │
├──────────────────────────────────────────────────────────────┤
│  systemd · NetworkManager · RAUC · nftables · timesyncd      │
│  journald (volatile + small persistent ring buffer)          │
├──────────────────────────────────────────────────────────────┤
│  Linux kernel (LTS; Raspberry Pi kernel fork on Pi)          │
└──────────────────────────────────────────────────────────────┘
```

## Build system

- **Yocto Project 6.0 "Wrynose" LTS** with layers:
  - `meta-castmill`: distro configuration (`DISTRO = "castmill"`), image recipes, agent, kiosk configuration
  - `meta-castmill-x86`: UEFI, systemd-boot, signed unified kernel image (UKI), Secure Boot, TPM
  - `meta-castmill-rpi`: Pi boot firmware, A/B boot logic, signed boot
- Upstream layers: `bitbake`, `openembedded-core`, `meta-yocto` (generic machines only), `meta-openembedded`, `meta-rauc`, `meta-raspberrypi`; `meta-intel` (x86-64, Intel media driver only); to evaluate: a layer providing Electron or Chromium (`meta-browser` has a Wrynose branch), compositor recipes (ADR-0004), `meta-security`.
- Builds are described with **kas** files so they are reproducible on any CI runner.
- See [10](10-build-release-testing.md).

## Image types

| Image | Contents | Use |
|---|---|---|
| `castmill-os-image` | Production rootfs | Installed into A/B slots |
| `castmill-os-bundle` | RAUC bundle of the rootfs | OTA updates |
| `castmill-os-installer` (x86) | Small installer that writes the disk image | USB / network install |
| `castmill-os-dev` | Production image plus debug tools, SSH, DevTools enabled (`kas/dev.yml` overlay) | Development only; never signed with production keys |

### Build targets

| Target | Machine | Purpose |
|---|---|---|
| `x86-64` | `genericx86-64` | Intel/AMD mini PCs |
| `rpi5` | `raspberrypi5` | Raspberry Pi 5 / CM5 |
| `vm-arm64` | `genericarm64` (UEFI) | Development VM for Apple Silicon Macs (QEMU/UTM); see [development guide](../development.md) |

## Disk layout

| Partition | Size (initial) | Filesystem | Mount | Notes |
|---|---|---|---|---|
| `boot` / ESP | 256 MB | vfat | `/boot` | Bootloader, UKIs or Pi firmware for both slots |
| `rootfs_a` | 1.5 GB | erofs or squashfs + dm-verity | `/` | Read-only |
| `rootfs_b` | 1.5 GB | erofs or squashfs + dm-verity | `/` | Read-only |
| `data` | rest | ext4 (LUKS2 when a TPM is present) | `/data` | Player cache, identity, configuration, logs |

Slot size leaves room for the image to grow. A slot is overwritten as a whole on update.

### Writable state

Everything writable lives under `/data`. Bind mounts and `systemd-tmpfiles` map it into place:

- `/data/castmill/player`: player cache and content
- `/data/castmill/identity`: device keys and certificate (unless in the TPM)
- `/data/castmill/config`: enrollment, network profiles, display configuration
- `/data/log`: small persistent log ring buffer
- `/etc` is read-only, except for a minimal overlay limited to specific files (for example NetworkManager connections stored under `/data`).

**Factory reset** reformats `data` (and generates a new encryption key). On the next boot the device is a fresh, unenrolled player.

## Boot flow

1. Firmware → bootloader with A/B slot selection and boot counting (systemd-boot boot counting on x86-64, `tryboot` or U-Boot bootcount on Pi; to be decided in the prototype).
2. Kernel + initramfs → dm-verity setup for the selected slot → unlock `data` (TPM) → switch root.
3. systemd → network, time sync, compositor, player, agent.
4. **Health check** (`castmill-health.service`, see [05](05-reliability.md)) → `rauc status mark-good` → boot counter reset.

## Runtime components

| Component | Responsibility | User |
|---|---|---|
| `sway` (kiosk config) | Wayland compositor; one fullscreen window per output; no shortcuts or launcher | `player` |
| `castmill-player` | Electron Castmill player; one BrowserWindow per output | `player` |
| `castmill-agent` | Enrollment, update orchestration (RAUC D-Bus), maintenance windows, health reporting, screenshots, logs, remote shell | `agent` (limited capabilities) |
| `castmill-health` | Post-boot and periodic health checks; commits or rejects slots | `root` (oneshot) |
| `rauc` | Installs and verifies bundles; manages slots | `root` |
| NetworkManager | Ethernet, Wi-Fi, (later) LTE; profiles from `/data` | `root` |

The player and the agent are deliberately separate, so device management keeps working if the player hangs or crashes (MGT-003).

## Player delivery

**Proposal (v1):** the Electron runtime and the player application are part of the OS image. One image version = one tested combination. Electron's own auto-updater is disabled.

Trade-off: a player hotfix needs an OS release. Adaptive/streamed RAUC updates keep that cheap, and the release pipeline has to support fast patch releases. To revisit if the player's release pace requires it.

## Interfaces

- **Agent ↔ Castmill backend:** outbound WebSocket over mutual TLS, plus HTTPS for bundles. See [07](07-device-management.md).
- **Player ↔ agent:** local Unix socket (status, display info, decode capabilities, "frame rendered" heartbeat).
- **Agent ↔ RAUC:** D-Bus (`de.pengutronix.rauc.Installer`).
- **Agent ↔ compositor:** compositor IPC for outputs and screenshots (Sway IPC + `grim`, or Weston equivalents; ADR-0004).
