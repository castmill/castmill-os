# 08 — Installation and provisioning

Status: Draft v0.1 · Related: PRV-*

## Goals

- Unboxed hardware to content on screen in **under 10 minutes**, with no keyboard needed.
- Hundreds of devices installable by non-specialist staff.

## Distribution formats

| Target | Artifact | Tool |
|---|---|---|
| Raspberry Pi 5 / CM5 | `castmill-os-rpi-<ver>.img.xz` + `.bmap` | Raspberry Pi Imager (custom repository entry), bmaptool, Etcher; `rpiboot` for CM5 eMMC |
| x86-64 | `castmill-os-x86-<ver>-installer.img` (USB) | Etcher / dd / Rufus |
| x86-64 (bulk) | Network-boot installer (UEFI HTTP boot / PXE) | Customer or partner infrastructure |
| x86-64 (factory) | Raw disk image + bmap | Pre-imaging by resellers |

## x86-64 USB installer

1. Boot from USB (UEFI).
2. The installer picks the internal disk automatically (largest non-removable disk, eMMC/NVMe preferred) and shows a **10-second countdown** with the disk model and size, then writes without asking. An "interactive" mode is available for multi-disk machines.
3. Writes the disk image with `bmaptool` (only used blocks), verifies it, creates the `data` partition across the rest of the disk, enrolls Secure Boot keys when the firmware allows it, and sets the boot order.
4. Copies a `castmill.conf` from the USB stick, if present (see below).
5. Powers off. A "done" screen tells the installer to remove the USB stick.

Target: under 3 minutes on NVMe or eMMC (PRV-001).

## First boot and enrollment

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

### Other keyboard-free configuration

- **Local setup access point** (MAY): with no network, the device starts a temporary Wi-Fi AP with a captive configuration page. Disabled once enrolled.
- **USB stick with `castmill.conf`** inserted at any time before enrollment.

## Re-provisioning

- Factory reset from Castmill, or locally with a documented hardware sequence (for example holding a key at boot on x86, or a `factory-reset` file on the Pi boot partition).
