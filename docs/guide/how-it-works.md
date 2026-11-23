# How it works

A tour of what runs on a Castmill OS device and why. The full design is in the [system architecture specification](../specs/02-system-architecture.md).

## The stack

```text
┌───────────────────────────────────────────────────────────────────┐
│ castmill-player@<output>          castmill-agent  (+ health check) │
│ kiosk shell, one per screen,      native side of the device app ·  │
│ runs Castmill's device app        A/B updates · provisioning       │
├───────────────────────────────────────────────────────────────────┤
│ Weston kiosk-shell (Wayland) · Mesa / KMS                          │
├───────────────────────────────────────────────────────────────────┤
│ systemd · NetworkManager · RAUC · nftables · journald · zram       │
├───────────────────────────────────────────────────────────────────┤
│ Linux kernel · read-only rootfs                                    │
├───────────────────────────────────────────────────────────────────┤
│ GRUB A/B with boot counting (UEFI) · Pi firmware tryboot A/B       │
└───────────────────────────────────────────────────────────────────┘
```

## The disk

Every target uses the same GPT layout (the Raspberry Pi has two more boot partitions):

| Partition | Contents | Mounted |
|---|---|---|
| `boot` | FAT. GRUB and its A/B state (UEFI), or the Pi's `autoboot.txt`. Holds `castmill.conf` and the `factory-reset` flag. | `/boot` |
| `boot_a`, `boot_b` | Raspberry Pi only, FAT: kernel, device trees and `config.txt` of each slot (the Pi firmware can't read the system partitions). | – |
| `rootfs_a` | The system, slot A. | `/`, read-only |
| `rootfs_b` | The system, slot B. Empty until the first update. | `/`, read-only |
| `data` | Everything that changes: pairing, settings, cache, logs. Grown to fill the disk on first boot. | `/data` |

The system partitions are never written while they're running. Everything a device learns lives on `data`, so a [factory reset](../operate/factory-reset.md) only has to recreate one partition.

## Boot

1. **Firmware and bootloader.** On UEFI machines GRUB picks the slot to boot from its A/B state and counts boot attempts. On the Raspberry Pi the firmware reads `autoboot.txt` and boots the committed slot, or the new slot once after an update (tryboot).
2. **Console banner.** The Castmill OS logo appears with the version, the machine and the slot.
3. **`castmill-data-setup`** checks `data`, grows it to the end of the disk the first time, and recreates it after a factory reset.
4. **`castmill-provision`** applies a [`castmill.conf`](../setup/castmill-conf.md) if there's one on the boot partition or a USB stick: Wi-Fi, proxy, NTP, time zone.
5. **`castmill-agent`** starts, works out the player URL and tells the kiosk shell about it.
6. **The display session** (Weston kiosk-shell) starts and launches **one kiosk-shell process per connected output**. Screens plugged in later are started on hotplug.
7. **`castmill-health`** decides whether an updated system is good. See [Updates and rollback](../operate/updates.md).

## On screen: Castmill's device app

Each kiosk-shell process loads **Castmill's device app** from your Castmill server (`https://castmill.net` by default). It's the same app every Castmill platform runs: it pairs with a pincode, plays content, and takes commands and sends telemetry over Castmill's channels.

What's special on Castmill OS is the **`castmillOS` bridge**. The kiosk shell exposes `window.castmillOS` to the device app, and Castmill's `CastmillOSMachine` uses it for native features:

- a stable device ID per screen, kept until a factory reset
- OS and hardware information, and telemetry (CPU, memory, storage, temperatures, network)
- reboot the device, restart the player, clear the cache
- settings and credentials stored on `data`

::: info Not Castmill's Electron player
The kiosk shell runs on Electron, but it isn't Castmill's desktop Electron player (`packages/platforms/electron-player`). It's a small shell, a few hundred lines with no npm dependencies, that loads the hosted device app. Electron is part of the OS image and updates with it. Native features come from `castmill-agent`, and each screen gets its own player process.
:::

The bridge only answers the top-level page of the Castmill server the device loaded. Iframes and other sites can't reach it. The details, including every method, are in [Castmill integration](../castmill-integration.md).

When the server can't be reached, the kiosk shell shows a local status page with the device name, the output, the OS version and the connection state, and tries again every 30 seconds. Once the device app has loaded, Castmill's service worker keeps it available offline.

## Self-healing

Castmill OS recovers from problems in layers, from the cheapest fix to the most drastic ([reliability specification](../specs/05-reliability.md)):

| Problem | Recovery |
|---|---|
| A screen stops rendering frames for 30 seconds | The agent reloads the page, then restarts that screen's player. |
| A player keeps crashing | systemd restarts it; a crash loop restarts the display session. |
| The display session keeps crashing | The device reboots. |
| The agent or the kernel hangs | The systemd and hardware watchdogs reset the device. |
| An update doesn't boot or fails its health check | The bootloader goes back to the previous slot after 3 attempts, or `castmill-health` rolls back. |
| Power is cut | The system is read-only and the agent writes its state atomically; logs are kept in memory, with a small persistent journal on `data`. |
| The network goes down | Content keeps playing. NetworkManager and the agent retry forever, and the clock is set from HTTPS when NTP is blocked. |

## Security

- Read-only system image; updates are signed RAUC bundles.
- No inbound ports: the firewall drops everything that the device didn't start. SSH exists only on development images.
- Every Castmill service runs as its own user with systemd sandboxing. polkit lets the agent reboot and restart the players, and nothing else.
- The kiosk shell runs with context isolation and the Chromium sandbox, no Node.js in pages, a navigation allowlist and permissions denied by default.

Verified boot (Secure Boot, dm-verity) and encryption of `data` are planned. See the [security specification](../specs/04-security.md).

## Build

Castmill OS is built with the [Yocto Project](https://www.yoctoproject.org/) 6.0 "Wrynose" LTS and [kas](https://kas.readthedocs.io/). The `meta-castmill` layer defines the distribution, the image, the update bundle and the system integration. GitHub Actions builds every target for every push and publishes a pre-release for every version tag. See the [development guide](../development.md).
