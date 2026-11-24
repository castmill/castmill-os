# Wi-Fi on site

Often the Wi-Fi password is only known once you're at the venue, or a venue changes its network. Castmill OS offers two ways to set it up there, without preparing the device in advance:

| | What you need | How |
|---|---|---|
| **On the screen** | A USB keyboard (and mouse), or a touchscreen | Pick the network on the screen and type the password |
| **USB stick** | A USB stick with a [`castmill.conf`](./castmill-conf.md) | Plug it into the device |

Both only work **while the device is offline**. A screen that is connected can't have its network changed by someone who plugs in a keyboard, touches it or plugs in a USB stick. To change the network of a working device, use [`castmill.conf`](./castmill-conf.md) on its boot partition.

## On the screen

Where you pick the network depends on whether the device has played Castmill content before.

### A new device: the status page

A device that has never loaded Castmill shows the Castmill OS status page. On devices with Wi-Fi it also shows the networks in range:

1. Plug in a USB keyboard and a mouse (or use the touchscreen).
2. Click the network, type its password and press **Connect**. For a hidden network, choose **Other network…** and type its name too.
3. The device connects and the status page shows "Connected". Within a few seconds the screen shows the pairing code.

### A device that has played before: the menu

Offline, a paired device keeps playing the content it has stored, so it doesn't show the status page. Use the player menu instead:

1. Plug in a USB keyboard (or use the touchscreen). Press any key, or touch the screen, to open the menu.
2. Choose **Wi-Fi** (arrow keys and Enter, or touch). It shows the networks in range.
3. Choose the network, type its password and press Enter. For a hidden network, choose **Other network…** and type its name first.
4. The menu shows "Connected to …", and the device goes back online with its content.

While the device is online, **Wi-Fi** only shows the network it's connected to.

If the password is wrong or the network is out of range, the screen says so and the device stays on its previous network.

The network is saved on the device's `data` partition: it's kept across reboots and updates, and erased by a [factory reset](../operate/factory-reset.md).

## USB stick

1. Put a `castmill.conf` with a `[network.wifi]` section in the root of a FAT or exFAT USB stick (see [castmill.conf](./castmill-conf.md#wi-fi)).
2. Plug the stick into the device. It doesn't need a reboot.
3. The device applies the file and connects. The file **stays on the stick**, so the same stick sets up the next device.

Keep the stick safe afterwards: the Wi-Fi password is in plain text on it.

## Doesn't work?

- **No networks listed, or no Wi-Fi in the menu:** the device has no Wi-Fi adapter, or the adapter needs firmware that isn't in the image ([hardware support](../specs/09-hardware-support.md)). Use Ethernet.
- **A network you expect is missing:** set the device's Wi-Fi country ([`wifi_country`](./castmill-conf.md#wi-fi-country), or its time zone): without one, some channels are only scanned passively.
- **The menu says the device is online:** it is, so the Wi-Fi can't be changed on site.
- **The stick is ignored:** the device is online, the file isn't called exactly `castmill.conf`, it isn't in the root of the stick, or it has an error (see [castmill.conf](./castmill-conf.md#how-its-applied)).
