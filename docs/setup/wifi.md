# Wi-Fi on site

Often the Wi-Fi password is only known once you're at the venue. Castmill OS offers two ways to set it up there, without preparing the device in advance:

| | What you need | How |
|---|---|---|
| **On the screen** | A USB keyboard (and mouse), or a touchscreen | Pick the network on the device's status screen and type the password |
| **USB stick** | A USB stick with a [`castmill.conf`](./castmill-conf.md) | Plug it into the device |

Both only work **while the device is offline**. A screen that is connected can't have its network changed by someone who plugs in a keyboard or a USB stick. To change the network of a working device, change it from somewhere it's already reachable, or use [`castmill.conf`](./castmill-conf.md) on its boot partition.

## On the screen

When the device can't reach Castmill, every screen shows the Castmill OS status page. On devices with Wi-Fi it also shows the networks in range:

1. Plug in a USB keyboard and a mouse (or use the touchscreen).
2. Click the network, type its password and press **Connect**. For a hidden network, choose **Other network…** and type its name too.
3. The device connects and the status page shows "Connected". Within a few seconds the screen loads Castmill: the pairing screen on a new device, the content on a paired one.

If the password is wrong or the network is out of range, the page says so and the device stays on its previous network.

The network is saved on the device's `data` partition: it's kept across reboots and updates, and erased by a [factory reset](../operate/factory-reset.md).

## USB stick

1. Put a `castmill.conf` with a `[network.wifi]` section in the root of a FAT or exFAT USB stick (see [castmill.conf](./castmill-conf.md#wi-fi)).
2. Plug the stick into the device. It doesn't need a reboot.
3. The device applies the file and connects. The file **stays on the stick**, so the same stick sets up the next device.

Keep the stick safe afterwards: the Wi-Fi password is in plain text on it.

## Doesn't work?

- **No networks listed:** the device has no Wi-Fi adapter, or the adapter needs firmware that isn't in the image ([hardware support](../specs/09-hardware-support.md)). Use Ethernet.
- **No status page, the content keeps playing:** the device is online, so the Wi-Fi can't be changed on site.
- **The stick is ignored:** the device is online, the file isn't called exactly `castmill.conf`, it isn't in the root of the stick, or it has an error (see [castmill.conf](./castmill-conf.md#how-its-applied)).
