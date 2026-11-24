# Introduction

Castmill OS is an open source **appliance operating system** for digital signage. It turns an Intel mini PC or a Raspberry Pi 5 into a [Castmill](https://castmill.io) player that runs unattended, 24/7, for years.

## Why another operating system?

For years Castmill players have run on general-purpose operating systems: Android, stock Linux distributions, Windows and iOS. None of them were made for screens that nobody looks after, and the same problems keep coming back:

- updates that break the player, or stop halfway and leave the device unbootable
- filesystem corruption after power cuts
- configuration drift between devices that should be identical
- screens that need a person on site to recover
- large attack surfaces and slow security patching
- video acceleration that works on one OS release and silently stops on the next

Castmill OS fixes these by design. It is one signed image, built for a single job, that updates itself safely and repairs itself.

## Design principles

| Principle | What it means on the device |
|---|---|
| **Immutable and atomic** | The root filesystem is read-only. Updates replace the whole system in an inactive A/B slot and switch to it atomically. |
| **Never needs a human on site** | Health-checked boots, automatic rollback, watchdogs at every layer, and storage that survives power loss. |
| **Secure by default** | Signed updates, no inbound ports, sandboxed services. Verified boot and disk encryption are on the roadmap. |
| **Fast media** | Hardware-accelerated graphics and video decoding, 4K content, several outputs. |
| **Installs at scale** | Flash in minutes, configure Wi-Fi and proxies with one file, pair with a pincode. |
| **Small** | Only what a signage player needs: no desktop, no package manager on the device. |

## What's in the box

- **Castmill's own device app** on every screen, loaded from your Castmill server. It pairs, plays content and reports to the dashboard exactly like the other Castmill players.
- **A kiosk shell** (Electron), one process per screen, that gives the device app native OS access through the `castmillOS` bridge.
- **`castmill-agent`**, a small Go service for the OS side: device identity, telemetry, A/B updates, health checks and provisioning.
- **Weston** (Wayland) as the display server, **RAUC** for updates, **systemd**, **NetworkManager** and **nftables**, built with the **Yocto Project** (6.0 "Wrynose" LTS).

Read [How it works](./how-it-works.md) for the details.

## Supported hardware

| Tier | Hardware | Status in v0.2 |
|---|---|---|
| 1 | Intel N100/N150-class mini PCs (x86-64, UEFI) | Image builds; A/B updates |
| 1 | Raspberry Pi 5 / Compute Module 5 | Image builds; A/B updates with the firmware's tryboot (to verify on a Pi) |
| dev | arm64 virtual machine (QEMU on Apple Silicon Macs, arm64 Linux) | Development image |
| 2 | Raspberry Pi 4, AMD mini PCs, Rockchip RK3588 | Later |

See [Hardware support](../specs/09-hardware-support.md) for requirements and the qualification checklist.

## Project status

Castmill OS is at **v0.2**, an early test release. The core works in the virtual machine (image, A/B updates with rollback, Castmill pairing, the native bridge); real hardware is being verified, but releases are signed with development keys and several security features are still to come. [Implementation status](../implementation-status.md) lists every requirement and how far it's verified, and the [roadmap](../specs/11-roadmap.md) shows what comes next.

## Next steps

- [Try it in a virtual machine on your Mac](./quick-start.md) in about five minutes.
- [Install it on a Raspberry Pi 5](../install/raspberry-pi-5.md) or [an Intel mini PC](../install/intel-x86-64.md).
- [Pair it with Castmill](../setup/pairing.md).
