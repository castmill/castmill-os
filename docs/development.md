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
- Enrollment, `castmill-agent`, remote commands, maintenance windows
- Player behaviour, layouts, multiple outputs, display hotplug (QEMU monitor)
- Network failure scenarios (QEMU network can be disconnected from the monitor)

### What it is **not** good for

- **Video decoding and GPU performance.** The VM uses `virtio-gpu`; it has none of the Pi's or Intel's video decoders. The player falls back to software rendering, which is fine for functional tests but says nothing about 4K playback.
- Secure Boot and TPM behaviour (can be emulated later with `swtpm`, but real hardware is the reference).

For GPU-accelerated graphics in the VM, UTM's QEMU build supports `virtio-gpu-gl` through ANGLE/Metal. It's optional and not needed for functional testing.

## Talk to the device: mock backend

The device needs a backend to pair with. For development, run the mock
implementation of the [device API](device-api.md) on your Mac:

```sh
cd agent
go run ./cmd/castmill-mock-backend --state ../.mock-backend --auto-approve \
  --bundles ../.vm/bundles
```

Then tell the development image to use it. Create `castmill.conf` on the
VM's boot partition (or in the image's `/boot` over SSH before the first
enrollment) with the mock's CA:

```toml
[backend]
url = "https://10.0.2.2:8443"     # the Mac, as seen from QEMU
ca_cert_file = "server-ca.crt"    # copy of .mock-backend/server-ca.crt, next to castmill.conf
```

The `[backend]` section only works on development images; production images
always use the built-in, pinned backend. `curl localhost:8081/devices` lists
the paired devices; see [device API](device-api.md#mock-backend-development)
for commands, configuration and updates.

### Test an update and a rollback

1. Build or download a second `vm-arm64` image with a different version and
   put its `castmill-os-bundle-genericarm64.raucb` in `.vm/bundles`.
2. Offer it: `curl -X POST localhost:8081/devices/<id>/desired -d '{"version":"<version>","bundle_url":"https://10.0.2.2:8443/bundles/castmill-os-bundle-genericarm64.raucb","activate_policy":"now"}'`.
3. Watch the serial console: install → reboot into slot B → `castmill-health`
   commits it (`castmill-agent status` on the device shows the result).
4. Rollback: offer a bundle whose player can't start (or stop
   `castmill-session.service` on the device during the trial boot). After the
   health timeout the device marks the slot bad, reboots and reports
   `rolled_back` with the reason.

Bundles are signed with the development keys of the build that produced them.
A device only accepts bundles signed by the keyring it was built with, so
use images and bundles from builds with the same `CASTMILL_KEYS_DIR`.

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
| `agent/` | `castmill-agent` (Go): enrollment, updates, health check, provisioning, commands | `cd agent && go test ./...` |
| `agent/cmd/castmill-mock-backend` | Development backend | `go run ./cmd/castmill-mock-backend` |
| `player/` | Electron kiosk shell (one process per output) | `cd player && npm run check` |
| `layers/meta-castmill/` | Yocto layer: distro, image, bundle, system integration | `kas-container shell kas/vm-arm64.yml -c "bitbake -g castmill-os-image"` |
| `scripts/` | VM runner, dev keys, Go module pin check | `scripts/check-go-mods.sh` |

Combine with `:` — for example `kas build kas/vm-arm64.yml:kas/dev.yml`.
