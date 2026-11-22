# Intel mini PCs (x86-64)

Install Castmill OS on an Intel or other x86-64 mini PC with UEFI firmware.

## What you need

| Part | Requirement |
|---|---|
| **CPU** | Intel N100, N150, N200 or Core i3/i5 12th gen and newer (reference hardware) |
| **Firmware** | UEFI. Legacy BIOS boot isn't supported. |
| **RAM** | 4 GB; 8 GB recommended for three screens |
| **Storage** | 32 GB or more: eMMC, NVMe or SSD |
| **Screens** | HDMI and DisplayPort, two or three outputs depending on the model |
| **USB stick** | 8 GB or more, to boot or install the image |

Other x86-64 machines with Intel or AMD graphics usually work too, but only the reference hardware is tested. See [Hardware support](../specs/09-hardware-support.md).

## 1. Prepare the firmware

Open the firmware setup (usually <kbd>Del</kbd>, <kbd>F2</kbd> or <kbd>Esc</kbd> at power-on) and set:

- **Secure Boot: disabled.** Castmill OS isn't signed for Secure Boot yet.
- **Boot mode: UEFI only.**
- **Restore on AC power loss: Power On.** The player starts by itself after a power cut. Strongly recommended for signage.

## 2. Flash the image

Download `castmill-os-image-genericx86-64.rootfs.wic.zst` from the [releases page](https://github.com/castmill/castmill-os/releases) and write it to a USB stick. See [Flashing tools](./index.md#flashing-tools).

```sh
sudo bmaptool copy castmill-os-image-genericx86-64.rootfs.wic.zst /dev/sdX
```

Need Wi-Fi? Add a [`castmill.conf`](../setup/castmill-conf.md) to the stick's **boot** partition now.

## 3. Install on the internal disk

There's no installer in v0.1. Pick one of these:

::: code-group

```text [Run from the USB stick]
Fastest way to try it. Boot the mini PC from the stick (boot menu:
usually F7, F11 or F12) and leave the stick in. Castmill OS runs
from it; the internal disk isn't touched.
```

```sh [Write the internal disk from a live Linux]
# Boot any live Linux USB (Ubuntu, Debian...) on the mini PC, then:
lsblk                         # find the internal disk, e.g. /dev/nvme0n1
sudo bmaptool copy castmill-os-image-genericx86-64.rootfs.wic.zst /dev/nvme0n1
# Remove the live USB and reboot.
```

```text [Flash the disk in another computer]
Take out the NVMe or SSD, connect it to your computer with a USB
adapter, flash it like a USB stick and put it back.
```

:::

A USB installer that writes the internal disk by itself in under three minutes is on the [roadmap](../specs/08-installation-and-provisioning.md#x86-64-usb-installer).

## 4. Boot

Power on. You'll see the Castmill OS logo in GRUB and on the console, with the version and the active slot, then Castmill's pairing screen on every connected screen. The first boot grows the `data` partition to fill the disk.

## 5. Pair

Claim each screen in the Castmill dashboard. See [Pair with Castmill](../setup/pairing.md).

## Updates

Intel devices have full A/B updates: a new version is installed in the background into the inactive slot and activated in the maintenance window, with automatic rollback if it doesn't come up healthy. See [Updates and rollback](../operate/updates.md).

## Limitations in v0.1

- Secure Boot, TPM-based disk encryption and dm-verity are planned, not available yet.
- **No hardware video decoding yet.** VA-API needs the Intel media driver, which isn't in the image yet. Video plays, decoded by the CPU.
