# Development guide

How to build Castmill OS and test it on your own machine before anything ships.

## Overview

| What | Where |
|---|---|
| Builds | GitHub Actions on larger runners (all targets, every push and internal PR) |
| Functional testing | **Development VM on your Mac** (`vm-arm64` target), or an arm64/x86-64 Linux machine |
| Media/performance testing | Real hardware: a Raspberry Pi 5 and an Intel mini PC on your desk, plus the hardware lab |

Yocto doesn't build natively on macOS, so the normal flow is: **push a branch → GitHub builds it → download the image → run it locally.**

## Requirements (macOS, Apple Silicon)

```sh
brew install qemu zstd gh
gh auth login
```

## Run the development VM

```sh
# Download the latest vm-arm64 image built from main
scripts/run-vm.sh fetch

# ...or the image from a specific workflow run (for example your branch's build)
scripts/run-vm.sh fetch 1234567890

# Boot it
scripts/run-vm.sh run

# Start again from a fresh, unenrolled device
scripts/run-vm.sh reset
```

The VM:

- boots through UEFI (the same boot flow as the x86-64 image), with **HVF hardware acceleration** on Apple Silicon, so it runs at near-native CPU speed
- has **2 virtual displays** by default (`CASTMILL_VM_OUTPUTS=1..4`) to test multi-output layouts
- shows the serial console in your terminal
- forwards SSH to `localhost:2222` (`ssh -p 2222 root@localhost`); CI always builds the VM as a development image
- keeps its disk in `.vm/` (git-ignored), so updates and rollbacks persist across restarts like on a real device

Settings: `CASTMILL_VM_MEM` (MB, default 4096), `CASTMILL_VM_CPUS` (default 4), `CASTMILL_VM_EXTRA_ARGS` (extra QEMU arguments).

### What the VM is good for

- Boot, health check, A/B updates and **rollback** (install a bundle, reboot, force a failure)
- Pairing with Castmill, `castmill-agent`, remote commands, maintenance windows
- Player behaviour, layouts, multiple outputs, display hotplug (QEMU monitor)
- Network failure scenarios (QEMU network can be disconnected from the monitor)

### What it is **not** good for

- **Video decoding and GPU performance.** The VM uses `virtio-gpu`; it has none of the Pi's or Intel's video decoders. The player falls back to software rendering, which is fine for functional tests but says nothing about 4K playback.
- Secure Boot and TPM behaviour (can be emulated later with `swtpm`, but real hardware is the reference).

For GPU-accelerated graphics in the VM, UTM's QEMU build supports `virtio-gpu-gl` through ANGLE/Metal. It's optional and not needed for functional testing.

## Pair the device with Castmill

Castmill OS runs Castmill's own device app, which pairs like any other Castmill player: it shows a pincode and a QR code, and you claim the screen in the Castmill dashboard. Each screen (output) is its own Castmill device. See [Castmill integration](castmill-integration.md).

By default devices use the production backend, `https://castmill.net`. To develop against a local Castmill backend (from the [Castmill repository](https://github.com/castmill/castmill), port 4000), point a development image at it with `castmill.conf` on the boot partition. The `[backend]` section is ignored on production images:

```sh
ssh -p 2222 root@localhost
# on the device (development images: root, empty password)
printf '[backend]\nurl = "http://10.0.2.2:4000"\n' > /boot/castmill.conf
systemctl restart castmill-provision castmill-agent castmill-session
```

`10.0.2.2` is your Mac as seen from QEMU.

### Test an update and a rollback

Until the Castmill backend manages OS versions, updates are requested on the device:

1. Download a newer `vm-arm64` release and serve its `castmill-os-bundle-genericarm64.raucb` from your Mac, for example with `python3 -m http.server 8000` in the download directory.
2. On the device: `castmill-agent update http://10.0.2.2:8000/castmill-os-bundle-genericarm64.raucb <version> --now`.
3. Watch the serial console: install, reboot into slot B, then `castmill-health` commits it. `castmill-agent status` shows the result.
4. Rollback: install a bundle whose player can't start, or stop `castmill-session.service` on the device during the trial boot. After the health timeout (10 minutes) the device marks the slot bad, reboots into slot A, and `castmill-agent status` shows `rolled_back` with the reason.

A device only accepts bundles signed by the keyring it was built with. Use images and bundles from CI, which shares one development key set across builds.

## Test on real hardware

Flash the image built by CI:

```sh
gh run download <run-id> --name image-rpi5 --dir out/
# Raspberry Pi 5 (SD card or USB/NVMe adapter); check the device name first!
sudo bmaptool copy out/castmill-os-image-raspberrypi5.rootfs.wic.zst /dev/diskN
```

For the Intel mini PC, use the `image-x86-64` artifact on a USB stick (the dedicated installer comes later, see spec 08).

## Build locally (optional)

Usually unnecessary: CI builds every branch. If you need a local build:

- Create development signing keys once: `scripts/gen-dev-keys.sh` (writes
  `build/castmill-dev-keys/`, git-ignored). Builds fail without them.
- **Linux workstation or server (x86-64 or arm64):** `pipx install kas` then `kas-container build kas/vm-arm64.yml:kas/dev.yml`. A first build needs 100+ GB of disk and a few hours; later builds reuse the cache.
- **macOS:** builds must run inside a Linux VM with its own **case-sensitive** disk (for example an OrbStack or Lima VM). Don't build in a folder shared from macOS: the macOS file system is case-insensitive by default, which Yocto doesn't support.

## Update the Yocto layers

Every target builds with exact layer commits from `kas/<target>.lock.yml`, so the same castmill-os commit always builds the same system. To move to newer commits of the layers' branches (security fixes in OE-core, for example):

```sh
uvx --with pyyaml python scripts/lock-layers.py           # rewrite the lock files
uvx --with pyyaml python scripts/lock-layers.py --check   # only report
```

Commit the changed `kas/*.lock.yml` on a branch and let CI build every target before merging. Do it at least once a month, and right away for security fixes.

## Targets

| kas file | Machine | Use |
|---|---|---|
| `kas/x86-64.yml` | `genericx86-64` | Intel/AMD mini PCs |
| `kas/rpi5.yml` | `raspberrypi5` | Raspberry Pi 5 / CM5 |
| `kas/vm-arm64.yml` | `genericarm64` | Development VM (Apple Silicon Macs, arm64 Linux) |
| `kas/dev.yml` | (overlay) | Development image: SSH, root login, debug tools, `castmill.conf` backend override |

## Components and their tests

| Path | What | Test locally |
|---|---|---|
| `agent/` | `castmill-agent` (Go): castmillOS bridge (native side), A/B updates, health check, provisioning | `cd agent && go test ./...` |
| `player/` | Electron kiosk shell (one process per output) running Castmill's device app | `cd player && npm run check` |
| `layers/meta-castmill/` | Yocto layer: distro, image, bundle, system integration | `kas-container shell kas/vm-arm64.yml -c "bitbake -g castmill-os-image"` |
| `scripts/` | VM runner, dev and production keys, Go module and layer pins | `scripts/check-go-mods.sh`, `scripts/lock-layers.py --check` |

Combine with `:` — for example `kas build kas/vm-arm64.yml:kas/dev.yml`.
