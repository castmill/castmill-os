# Intel mini PCs (x86-64)

Install Castmill OS on an Intel or other x86-64 mini PC with UEFI firmware.

## What you need

| Part | Requirement |
|---|---|
| **CPU** | Intel N100, N150, N200 or Core i3/i5 12th gen and newer (reference hardware) |
| **Firmware** | UEFI. Legacy BIOS boot isn't supported. |
| **RAM** | 4 GB; 8 GB recommended for three screens |
| **Storage** | 32 GB or more: eMMC, NVMe or SSD. Barebone kits (many Intel NUCs) come without a disk or RAM: add an NVMe SSD. |
| **Screens** | HDMI and DisplayPort, two or three outputs depending on the model |
| **USB stick** | 2 GB or more, for the installer. It's only needed during installation and can install any number of PCs. |

Other x86-64 machines with Intel or AMD graphics usually work too, but only the reference hardware is tested. See [Hardware support](../specs/09-hardware-support.md).

## 1. Prepare the firmware

Open the firmware setup (usually <kbd>Del</kbd>, <kbd>F2</kbd> or <kbd>Esc</kbd> at power-on) and set:

- **Secure Boot: disabled.** Castmill OS isn't signed for Secure Boot yet.
- **Boot mode: UEFI only.**
- **Restore on AC power loss: Power On.** The player starts by itself after a power cut. Strongly recommended for signage.

## 2. Make the installer stick

Download `castmill-os-installer-genericx86-64.img.xz` from the [releases page](https://github.com/castmill/castmill-os/releases) and write it to a USB stick. Everything on the stick is erased.

::: code-group

```text [Windows / macOS / Linux]
Raspberry Pi Imager → Choose OS → Use custom → the .img.xz file
→ Choose storage → the USB stick → Write

(balenaEtcher and Rufus work too.)
```

```sh [Linux (bmaptool)]
lsblk                                            # find the stick, e.g. /dev/sdb
sudo bmaptool copy castmill-os-installer-genericx86-64.img.xz /dev/sdX
```

:::

After writing, your computer shows a drive called **CASTMILL**. Need Wi-Fi, a proxy or a time zone? Copy `castmill.conf.example` to `castmill.conf` on that drive and edit it ([reference](../setup/castmill-conf.md)). On Windows, check that the name doesn't end in `.txt`. The installer copies it to every PC it installs.

## 3. Install

1. Plug the stick into the PC and power it on. Open the boot menu (usually <kbd>F7</kbd>, <kbd>F10</kbd>, <kbd>F11</kbd> or <kbd>F12</kbd>; on Intel NUCs <kbd>F10</kbd>) and choose the USB stick.
2. The installer shows the internal disk it will use, with its model and size, and starts after **10 seconds**. **Everything on that disk is erased**, including any Windows installation.
   - To use another disk, or to cancel, press any key during the countdown. A menu lists the disks.
   - With a single disk there's nothing to do: no keyboard is needed.
3. It writes Castmill OS (a progress percentage shows) and copies `castmill.conf`. That takes one to three minutes.
4. The PC powers off. **Remove the stick.**

The same stick installs any number of PCs.

::: details Which disk does the installer pick?
NVMe first, then eMMC, then SATA, and the largest disk of the best kind. It never picks the stick itself or other USB disks. If the PC has no internal disk, the menu lets you install to a USB SSD instead. A cheap USB stick isn't a good place to run Castmill OS: playing 24/7 wears it out.
:::

::: details Without the installer
You can also write the disk image `castmill-os-image-genericx86-64.rootfs.wic.zst` to the internal disk directly: take the SSD out and flash it with a USB adapter, or boot a live Linux on the PC and use `bmaptool copy` on the internal disk.
:::

## 4. Boot

Power on (without the stick). You'll see the Castmill OS logo in GRUB and on the console, with the version and the active slot, then Castmill's pairing screen on every connected screen. The first boot grows the `data` partition to fill the disk.

## 5. Pair

Claim each screen in the Castmill dashboard. See [Pair with Castmill](../setup/pairing.md).

## Updates

Intel devices have full A/B updates: a new version is installed in the background into the inactive slot and activated in the maintenance window, with automatic rollback if it doesn't come up healthy. See [Updates and rollback](../operate/updates.md).

## Limitations in v0.1

- Secure Boot, TPM-based disk encryption and dm-verity are planned, not available yet.
- **No hardware video decoding yet.** VA-API needs the Intel media driver, which isn't in the image yet. Video plays, decoded by the CPU.
