# Pair with Castmill

Castmill OS runs Castmill's own device app, so a Castmill OS screen pairs exactly like any other Castmill player.

## Pair a screen

1. **Boot the device.** Each connected screen shows Castmill's pairing screen with a **pincode** and a **QR code**.
2. **In the Castmill dashboard**, add a device and enter the pincode, or scan the QR code with your phone.
3. **Finish in the dashboard:** name the device and assign it the content to play.

Content starts within seconds. The device keeps its pairing across reboots and updates, until a [factory reset](../operate/factory-reset.md).

## Several screens on one device

**Each screen is its own Castmill device.** A mini PC with three screens shows three pincodes, and you pair each one separately. That way every screen gets its own content, schedule and status in the dashboard, and a screen can be replaced or moved without touching the others.

Each screen has a device ID made from the device's ID and the output name (for example `…-HDMI-A-1`), so a screen keeps its pairing as long as it stays on the same port.

## Which Castmill server?

Production images connect to **`https://castmill.net`**. Images built for your own Castmill installation can use another address (`CASTMILL_BACKEND_URL` at build time).

[Development images](../install/virtual-machine.md#development-and-production-images) can be pointed at any server with `castmill.conf`, for example a Castmill backend on your laptop:

```toml
[backend]
url = "http://10.0.2.2:4000"       # your Mac, as seen from the QEMU VM
```

See the [development guide](../development.md#pair-the-device-with-castmill).

## What the dashboard shows

With Castmill OS support in Castmill's device app (`CastmillOSMachine`), the dashboard gets:

| Feature | Details |
|---|---|
| **Device info** | App type "Castmill OS", OS version, hardware model, Chromium and Electron versions |
| **Telemetry** | CPU usage and load, memory, storage of the `data` partition, temperatures, network type and signal, uptime |
| **Commands** | Restart the player, reboot the device, clear the cache, refresh |

::: info Older Castmill servers
If the Castmill server you use doesn't have `CastmillOSMachine` yet, screens still pair and play, but as plain web players without the native features.
:::

## When a screen can't reach Castmill

Instead of the pairing screen, the screen shows a local **status page**:

- the device name, the output and the Castmill OS version
- whether Castmill can be reached, and the last error
- `DEVELOPMENT IMAGE` on development images

The device tries again every 30 seconds. Check the network, and any [proxy settings](./castmill-conf.md#proxy). Already-paired screens keep playing their cached content while offline.
