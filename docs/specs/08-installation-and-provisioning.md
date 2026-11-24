# 08 — Installation and provisioning

Status: v0.3 · Related: PRV-*

## Goals

- Unboxed hardware to content on screen in **under 10 minutes**, with no keyboard needed.
- Hundreds of devices installable by non-specialist staff.

## Distribution formats

| Target | Artifact | Tool |
|---|---|---|
| Raspberry Pi 5 / CM5 | `castmill-os-rpi-<ver>.img.xz` + `.bmap` | Raspberry Pi Imager (custom repository entry), bmaptool, Etcher; `rpiboot` for CM5 eMMC |
| x86-64 | `castmill-os-installer-genericx86-64.img.xz` + `.img.bmap` (USB stick) | Raspberry Pi Imager, balenaEtcher, Rufus, bmaptool |
| x86-64 (bulk) | Network-boot installer (UEFI HTTP boot / PXE) | Customer or partner infrastructure |
| x86-64 (factory) | Raw disk image + bmap | Pre-imaging by resellers |

## x86-64 USB installer

1. Boot from USB (UEFI).
2. The installer checks the image on the stick, then picks the internal disk automatically: NVMe first, then eMMC, then SATA, the largest of the best kind; never the stick itself or another USB disk. It shows a **10-second countdown** with the disk model and size, then writes without asking. Pressing any key opens a menu to pick another disk (USB disks included, for PCs without an internal disk) or cancel.
3. Writes the disk image with a progress percentage. The `data` partition is grown across the rest of the disk on the first boot (`castmill-data-setup`). Planned: enrolling Secure Boot keys when the firmware allows it, and setting the boot order (today the firmware boots the disk through its default `EFI/BOOT` path).
4. Copies a `castmill.conf` (and the certificates next to it) from the stick, if present (see below).
5. Shows a "done" screen telling the user to remove the stick, and powers off.

The stick has three partitions: an ESP with GRUB and the kernel, a read-only installer system (BusyBox, `zstd` and `castmill-install` as init), and a FAT partition labelled `CASTMILL`. Windows and macOS show the `CASTMILL` partition as a drive; it holds the Castmill OS image, `README.txt` and `castmill.conf.example`.

Target: under 3 minutes on NVMe or eMMC (PRV-001).

## First boot and enrollment

> **Revised by [ADR-0008](../adr/0008-reuse-castmill-device-stack.md):** pairing uses Castmill's pincode and QR code, shown by Castmill's device app; each screen is its own Castmill device. Zero-touch bulk enrollment with an organization token (`[enrollment]` in `castmill.conf`) needs Castmill support and is not available yet.

1. The device boots and expands or creates `data`.
2. Network: Ethernet with DHCP works with no setup. For Wi-Fi, see the configuration options below.
3. The agent generates the device identity and contacts Castmill.
4. The screen shows a **pairing code and QR code**. The operator enters or scans it in Castmill and assigns the device to an organization and group.
5. The device receives its certificate and configuration, and content starts.

### Zero-touch bulk enrollment

A `castmill.conf` file on the boot partition (Pi) or the installer stick (x86):

```toml
[enrollment]
org_token = "…"            # one-time or limited-use token from Castmill
group = "stores/madrid"
name_template = "{group}-{serial}"

[network.wifi]
ssid = "…"
psk = "…"

[network.proxy]
https = "http://proxy.example:3128"
ca_cert_file = "proxy-ca.pem"

[time]
ntp = ["ntp.example.com"]
timezone = "Europe/Madrid"
```

The file is used once, moved into encrypted `data`, and deleted from the boot partition.

### Configuration on site

Both only while the device is offline, so a working screen can't be reconfigured by whoever plugs something in ([Wi-Fi on site](../setup/wifi.md)):

- **On-screen Wi-Fi setup:** the status page lists the networks in range; pick one and type the password with a USB keyboard and mouse or a touchscreen.
- **USB stick with `castmill.conf`:** applied when plugged in, and kept on the stick for the next device.
- *Not planned for now:* a temporary setup access point with a captive page, and TV-remote (HDMI-CEC) control of the status page with an on-screen keyboard.

## Re-provisioning

- Factory reset from Castmill, or locally with a documented hardware sequence (for example holding a key at boot on x86, or a `factory-reset` file on the Pi boot partition).
