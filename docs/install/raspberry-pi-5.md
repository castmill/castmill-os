# Raspberry Pi 5

Install Castmill OS on a Raspberry Pi 5 or a Compute Module 5.

## What you need

| Part | What to use |
|---|---|
| **Board** | Raspberry Pi 5 with 4 GB RAM (2 GB minimum), or a CM5 on a carrier board |
| **Storage** | NVMe SSD on an M.2 HAT (recommended), a high-endurance microSD card, or a USB SSD. 16 GB or more. |
| **Power** | The **official 27 W USB-C power supply**. Undervoltage is the most common cause of failures in the field. |
| **Screens** | One or two, on the micro-HDMI ports, up to 4K60 each |
| **Network** | Ethernet, or Wi-Fi configured with `castmill.conf` |
| **Cooling** | The Active Cooler or a case with a fan, for screens running 24/7 |

## 1. Flash the image

Download `castmill-os-image-raspberrypi5.rootfs.wic.zst` from the [releases page](https://github.com/castmill/castmill-os/releases) and write it to the SD card or SSD (connect the SSD with a USB adapter). See [Flashing tools](./index.md#flashing-tools) for Linux, macOS and Windows.

```sh
sudo bmaptool copy castmill-os-image-raspberrypi5.rootfs.wic.zst /dev/sdX
```

::: details Compute Module 5 with eMMC
Put the CM5 in USB boot mode on its carrier board, run [`rpiboot`](https://github.com/raspberrypi/usbboot) to expose the eMMC as a USB drive, then flash it like any disk.
:::

## 2. Configure Wi-Fi (optional)

After flashing, your computer shows the FAT partition called **boot**. Create a file called `castmill.conf` in it:

```toml
[network.wifi]
ssid = "Store-Signage"
psk = "your-wifi-password"

[time]
timezone = "Europe/Madrid"
```

The device applies the file on its next boot and then removes it from the boot partition, so the password doesn't stay there in plain text. All the options are in the [`castmill.conf` reference](../setup/castmill-conf.md).

## 3. Boot

1. Connect the screens to the micro-HDMI ports and plug in the network cable if you use Ethernet.
2. Insert the SD card or connect the SSD, then connect the power supply.
3. The Castmill OS logo appears on the first screen during boot, then each screen shows Castmill's pairing screen.

The first boot takes a little longer because it grows the `data` partition to fill the disk.

::: tip Booting from NVMe
Recent Raspberry Pi bootloaders try the SD card first, then NVMe, then USB. If the Pi doesn't boot from your SSD, remove the SD card and update the bootloader: Raspberry Pi Imager → *Misc utility images* → *Bootloader* → *NVMe/USB boot*.
:::

## 4. Pair

Claim each screen in the Castmill dashboard with its pincode. See [Pair with Castmill](../setup/pairing.md).

## Limitations in v0.1

- **No A/B updates on the Pi yet.** The Pi always boots slot A, and the image doesn't include RAUC until A/B boot is implemented for the Pi firmware. To move to a new version, flash the new image; the device then has to be paired again.
- **No boot logo in the bootloader.** The logo shows on the console once Linux starts.
- **Signed boot** (the Pi's equivalent of Secure Boot) is planned.

See the [roadmap](../specs/11-roadmap.md) for when these arrive.
