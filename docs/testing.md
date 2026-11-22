# Testing a Castmill OS release

How to try a Castmill OS test release (`v0.x`) and what to check. Releases are on the [GitHub releases page](https://github.com/castmill/castmill-os/releases); each one has the images, RAUC update bundles, SBOMs and a `SHA256SUMS` file.

> Test releases are signed with **development keys**. Don't install them on screens in production.

## What each image can do today

| Image | Use | Pairing |
|---|---|---|
| `genericarm64` (development VM) | Functional testing on a Mac or arm64 Linux machine: pairing, commands, updates, rollback | With the mock backend on your machine |
| `genericx86-64` (Intel mini PCs) | Boot, display, network, hardware support | **Not yet**: production images only pair with `https://castmill.net`, which doesn't serve the [device API](device-api.md) yet |
| `raspberrypi5` | Same as x86-64; no A/B updates yet | **Not yet** (same reason) |

## Development VM (full flow)

Requirements: an Apple Silicon Mac (or arm64 Linux), `brew install qemu zstd gh`, Go 1.24+, and a clone of this repository.

1. **Download and boot the image**

   ```sh
   gh release download v0.1.0 -R castmill/castmill-os -p '*genericarm64*' -D .vm/download
   scripts/run-vm.sh run .vm/download/castmill-os-image-genericarm64.rootfs.wic.zst
   ```

   A QEMU window opens (the VM has two virtual screens) and the serial console stays in your terminal. You should see the Castmill OS logo in the console, then the status screen saying "Contacting Castmill…".

2. **Start the mock backend** (second terminal):

   ```sh
   cd agent && go run ./cmd/castmill-mock-backend --state ../.mock-backend
   ```

3. **Point the VM at it.** The development image accepts a backend override in `castmill.conf`:

   ```sh
   scp -P 2222 .mock-backend/server-ca.crt root@localhost:/boot/
   ssh -p 2222 root@localhost 'cat > /boot/castmill.conf <<EOF
   [backend]
   url = "https://10.0.2.2:8443"
   ca_cert_file = "server-ca.crt"
   EOF
   systemctl restart castmill-provision castmill-agent'
   ```

   (The root password is empty on development images.) The screens now show a pairing code.

4. **Pair it** with the code from the screen:

   ```sh
   curl -X POST localhost:8081/pair -d '{"code":"ABC-123"}'
   curl -s localhost:8081/devices | jq '.[0] | {id, online, status: .status.os_version}'
   ```

   The screens switch to the player URL (the default player URL is a placeholder, so expect a load error page that retries).

5. **Try the commands** (replace `dev-0002` with your device ID):

   ```sh
   D=dev-0002
   curl -X POST localhost:8081/devices/$D/command -d '{"command":"screenshot"}'      # PNG in .mock-backend/uploads/
   curl -X POST localhost:8081/devices/$D/config  -d '{"player_url":"https://example.com/"}'
   curl -X POST localhost:8081/devices/$D/config  -d '{"outputs":{"Virtual-1":{"transform":"rotate-90"}}}'
   curl -X POST localhost:8081/devices/$D/command -d '{"command":"collect_logs"}'
   curl -X POST localhost:8081/devices/$D/command -d '{"command":"reboot"}'
   ```

6. **Update and roll back**: see the [development guide](development.md#test-an-update-and-a-rollback). Use the `.raucb` bundle from a newer release; bundles and images from the same CI signing keys are compatible.

`scripts/run-vm.sh reset` starts again from a fresh, unenrolled device.

## Hardware (x86-64 mini PC, Raspberry Pi 5)

Flash the image to a USB stick (x86-64; boot the PC from it, there's no installer yet) or an SD card / NVMe drive (Pi 5). Check the device name first:

```sh
sudo bmaptool copy castmill-os-image-<machine>.rootfs.wic.zst /dev/<disk>
```

## Checklist

| Check | VM | x86-64 | Pi 5 |
|---|---|---|---|
| Boot logo in GRUB / on the console, version and slot in the banner | ✓ | ✓ | console only |
| Pairing screen on every connected output | ✓ | ✓ | ✓ |
| Ethernet works without configuration | ✓ | ✓ | ✓ |
| Wi-Fi from `castmill.conf` on the boot partition | – | ✓ | ✓ |
| Display hotplug: unplug and replug a screen | – | ✓ | ✓ |
| Power cut during boot: device comes back | ✓ | ✓ | ✓ |
| Pairing, screenshot, config, logs, reboot | ✓ | – | – |
| Update and rollback | ✓ | – | – |

Report problems as GitHub issues with the release version (shown in the console banner), the image, what you did and, if possible, `castmill-agent status` output (development VM: over SSH).
