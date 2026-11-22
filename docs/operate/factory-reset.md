# Factory reset

A factory reset returns a device to the state it was in when it was flashed. The system slots aren't touched; only the `data` partition is recreated.

## What's erased

Everything on `data`:

- the Castmill pairing of every screen (credentials and settings)
- the device ID: the device comes back as a new device
- applied `castmill.conf` settings: Wi-Fi, proxy, NTP, time zone
- the content cache and the persistent logs

The installed Castmill OS version stays the same.

## How to reset

Create an empty file called **`factory-reset`** in the root of the **boot** partition and reboot:

- **Raspberry Pi:** take out the SD card (or connect the SSD to your computer), create the file on the `boot` partition and put it back.
- **Intel mini PC:** connect the disk or USB stick to your computer, or use a live Linux USB, and create the file on the `boot` partition.
- **Development images:** over SSH, `touch /boot/factory-reset && reboot`.

At the next boot `castmill-data-setup` deletes the flag and recreates `data`, and every screen shows a new pincode. Remove the old devices from the Castmill dashboard and pair the screens again.

::: tip Reconfigure in the same step
Put a new [`castmill.conf`](../setup/castmill-conf.md) next to the `factory-reset` file: the reset runs first, then the new configuration is applied.
:::

Remote factory reset from the Castmill dashboard is planned.
