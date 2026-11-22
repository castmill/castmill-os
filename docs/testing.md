# Testing a Castmill OS release

How to try a Castmill OS test release (`v0.x`) and what to check. Releases are on the [GitHub releases page](https://github.com/castmill/castmill-os/releases); each one has the images, RAUC update bundles, SBOMs and a `SHA256SUMS` file.

> Test releases are signed with **development keys**. Don't install them on screens in production.

## How pairing works

Castmill OS runs Castmill's own device app ([Castmill integration](castmill-integration.md)). Every screen shows a Castmill pincode and QR code; claim it in the Castmill dashboard like any other player. Each screen of a multi-screen device is a separate Castmill device.

- **Production images** (x86-64, Raspberry Pi 5) use `https://castmill.net`.
- **The development VM image** uses `https://castmill.net` too, but can be pointed at a local Castmill backend (see the [development guide](development.md#pair-the-device-with-castmill)).

Native features on Castmill OS need the `CastmillOSMachine` support in Castmill's device app: a stable device ID, OS info and telemetry, reboot and restart from the dashboard. Until that's deployed on the backend you use, devices still pair and play, but as plain web players.

## Development VM

Requirements: an Apple Silicon Mac (or arm64 Linux), `brew install qemu zstd gh`, and a clone of this repository.

```sh
gh release download v0.1.0 -R castmill/castmill-os -p '*genericarm64*' -D .vm/download
scripts/run-vm.sh run .vm/download/castmill-os-image-genericarm64.rootfs.wic.zst
```

A QEMU window opens (the VM has two virtual screens) and the serial console stays in your terminal. You should see the Castmill OS logo in the console, then Castmill's pairing screen. `scripts/run-vm.sh reset` starts again from a fresh, unpaired device.

## Hardware (x86-64 mini PC, Raspberry Pi 5)

Flash the image to a USB stick (x86-64; boot the PC from it, there's no installer yet) or an SD card or NVMe drive (Pi 5). Check the device name first:

```sh
sudo bmaptool copy castmill-os-image-<machine>.rootfs.wic.zst /dev/<disk>
```

Wi-Fi: put a `castmill.conf` with a `[network.wifi]` section on the boot partition before the first boot (see [spec 08](specs/08-installation-and-provisioning.md)). Ethernet works without configuration.

## Checklist

| Check | VM | x86-64 | Pi 5 |
|---|---|---|---|
| Boot logo in GRUB / on the console, version and slot in the banner | ✓ | ✓ | console only |
| Castmill pairing screen on every connected output | ✓ | ✓ | ✓ |
| Claim in the dashboard; content plays on each screen | ✓ | ✓ | ✓ |
| Dashboard: device info shows "Castmill OS", telemetry works* | ✓ | ✓ | ✓ |
| Dashboard: restart app and reboot device* | ✓ | ✓ | ✓ |
| Ethernet works without configuration | ✓ | ✓ | ✓ |
| Wi-Fi from `castmill.conf` on the boot partition | – | ✓ | ✓ |
| Unplug the network: content keeps playing, the device recovers when it's back | ✓ | ✓ | ✓ |
| Display hotplug: unplug and replug a screen | – | ✓ | ✓ |
| Power cut during boot: the device comes back | ✓ | ✓ | ✓ |
| Update and rollback ([development guide](development.md#test-an-update-and-a-rollback)) | ✓ | ✓ | – |

\* Needs `CastmillOSMachine` deployed on the backend.

Report problems as GitHub issues with the release version (shown in the console banner), the image, what you did and, if possible, `castmill-agent status` output (development VM: over SSH, `ssh -p 2222 root@localhost`).
