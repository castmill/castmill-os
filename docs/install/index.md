# Install Castmill OS

Castmill OS ships as a ready-to-flash disk image for each kind of hardware. You write it to the device's disk (or an SD card or USB stick), optionally add a `castmill.conf` for Wi-Fi, and power on.

## Pick your hardware

<div class="install-cards">

[**Raspberry Pi 5** <br> SD card, NVMe or USB. Two 4K screens.](./raspberry-pi-5.md)

[**Intel mini PC** <br> N100/N150-class x86-64 with UEFI. Two or three screens.](./intel-x86-64.md)

[**Virtual machine** <br> Apple Silicon Mac or arm64 Linux. For trying and development.](./virtual-machine.md)

</div>

## Downloads

Every version is published on the [GitHub releases page](https://github.com/castmill/castmill-os/releases):

| File | What it is |
|---|---|
| `castmill-os-image-raspberrypi5.rootfs.wic.zst` | Raspberry Pi 5 / CM5 disk image |
| `castmill-os-image-genericx86-64.rootfs.wic.zst` | Intel / x86-64 disk image |
| `castmill-os-image-genericarm64.rootfs.wic.zst` | arm64 development VM image (SSH, root login) |
| `*.wic.bmap` | Block map, so `bmaptool` writes only the used blocks |
| `castmill-os-bundle-<machine>.raucb` | Signed update bundle for devices already running Castmill OS |
| `*.spdx.json` | Software bill of materials |
| `SHA256SUMS` | Checksums of every file |

The latest development build of `main` is also available as GitHub Actions artifacts (`image-rpi5`, `image-x86-64`, `image-vm-arm64`).

::: warning Test releases
Releases are currently signed with **development keys** and are meant for testing. Don't install them on screens in production yet.
:::

### Check the download

```sh
gh release download v0.1.0 -R castmill/castmill-os -p '*raspberrypi5*' -p SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS      # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS
```

## Flashing tools

Use whichever you have. All of them erase the target disk, so **check the disk name twice**.

::: code-group

```sh [Linux (bmaptool)]
lsblk                                            # find the disk, e.g. /dev/sdb
sudo bmaptool copy castmill-os-image-<machine>.rootfs.wic.zst /dev/sdX
```

```sh [macOS]
diskutil list                                    # find the disk, e.g. /dev/disk4
diskutil unmountDisk /dev/diskN
zstd -dc castmill-os-image-<machine>.rootfs.wic.zst | sudo dd of=/dev/rdiskN bs=4m
diskutil eject /dev/diskN
```

```text [Windows / any OS]
Raspberry Pi Imager → Choose OS → Use custom → pick the image
(decompress it first with zstd -d if Imager doesn't accept .zst)
→ Choose storage → Write
```

:::

`bmaptool` is the fastest because it skips empty blocks; it reads the `.bmap` file next to the image automatically.

## After flashing

1. **Wi-Fi or a proxy?** Copy a [`castmill.conf`](../setup/castmill-conf.md) to the `boot` partition. Ethernet with DHCP needs no configuration.
2. **Boot the device.** The first boot grows the `data` partition to fill the disk.
3. **Pair it.** Each screen shows a pincode; claim it in the Castmill dashboard. See [Pair with Castmill](../setup/pairing.md).

<style>
.install-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  margin: 16px 0 8px;
}
.install-cards p {
  margin: 0;
  display: contents;
}
.vp-doc .install-cards a {
  display: block;
  padding: 16px 18px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
  text-decoration: none;
  font-size: 14px;
  line-height: 1.5;
  transition: border-color 0.2s;
}
.vp-doc .install-cards a strong {
  color: var(--vp-c-text-1);
  font-size: 16px;
}
.vp-doc .install-cards a:hover {
  border-color: var(--vp-c-brand-1);
}
</style>
