# castmill.conf

`castmill.conf` configures a device without a keyboard: Wi-Fi, proxy, time servers and time zone. It's a [TOML](https://toml.io) file that you put on the device's **boot** partition before it boots.

## How it's applied

**On the boot partition:**

1. Put `castmill.conf` in the root of the **boot** partition (the FAT partition your computer shows after flashing).
2. At boot, `castmill-provision` applies it before the network and the agent start.
3. The file is then moved to `/data` and **deleted from the boot partition**, so Wi-Fi passwords don't stay there in plain text.

It works on every boot, not just the first: to change the Wi-Fi of a device in the field, put a new `castmill.conf` on its boot partition and reboot.

**On a USB stick:** plug a FAT or exFAT stick with `castmill.conf` in its root into the device. It's applied right away, **only while the device is offline**, and the file stays on the stick for the next device. See [Wi-Fi on site](./wifi.md), which also explains the on-screen setup.

::: warning Typos stop the file
Unknown keys are rejected so a typo can't leave a device silently unconfigured. If the file has an error, nothing in it is applied, it stays on the boot partition so you can fix it, and the error is logged by `castmill-provision`.
:::

## Example

```toml
[network]
wifi_country = "ES"                # optional: the time zone's country by default

[network.wifi]
ssid = "Store-Signage"
psk = "your-wifi-password"

[network.proxy]
https = "http://proxy.example.com:3128"
no_proxy = "localhost,127.0.0.1,.example.com"
ca_cert_file = "proxy-ca.pem"      # next to castmill.conf

[time]
ntp = ["ntp1.example.com", "ntp2.example.com"]
timezone = "Europe/Madrid"
```

## Reference

### Wi-Fi

Section `[network.wifi]`.

| Key | Type | Description |
|---|---|---|
| `ssid` | string, required | Network name |
| `psk` | string | WPA/WPA2 passphrase. Leave it out for open networks. |
| `hidden` | boolean | `true` if the network doesn't broadcast its name |

The device creates a NetworkManager connection that's tried automatically. Ethernet keeps working and needs no configuration.

#### Wi-Fi country

Key `wifi_country` in section `[network]`: the two-letter code (ISO 3166) of the country the device is in, like `"SE"` or `"ES"`. A code that isn't a country is rejected, like other mistakes in the file. Wi-Fi channels and power limits differ by country. Without one, Wi-Fi follows the strictest worldwide rules, and networks on some channels (for example 2.4 GHz channels 12 and 13, common in Europe) show up late or not at all.

You rarely need it: by default the device uses the country of its time zone (from `[time]` or from Castmill, aliases like `US/Eastern` included), applied at the next boot. A time zone without a country, like `UTC`, gives the worldwide rules. Set it when the time zone doesn't match the country, or when the device should get it right before its first connection.

### Proxy

Section `[network.proxy]`.

| Key | Type | Description |
|---|---|---|
| `http` | URL | Proxy for `http://` |
| `https` | URL | Proxy for `https://`, which is what Castmill uses |
| `no_proxy` | string | Comma-separated hosts and domains that bypass the proxy |
| `ca_cert_file` | path | PEM certificate of a TLS-inspecting proxy, relative to `castmill.conf` |

The agent and the players use the proxy.

### Time

Section `[time]`.

| Key | Type | Description |
|---|---|---|
| `ntp` | list of strings | NTP servers to use instead of the defaults |
| `timezone` | string | IANA time zone, such as `Europe/Madrid` or `America/New_York`. Used for schedules and the maintenance window. |

When NTP is blocked, the device sets its clock from the `Date` header of Castmill's HTTPS responses.

### Backend (development images only)

Section `[backend]`.

| Key | Type | Description |
|---|---|---|
| `url` | URL | Castmill server to use instead of `https://castmill.net` |
| `ca_cert_file` | path | PEM certificate of a private CA for that server |

Production images ignore this section and log a warning.

### Enrollment (reserved)

Section `[enrollment]`.

`org_token`, `group` and `name_template` are accepted for zero-touch bulk enrollment, which needs support in Castmill and doesn't work yet. Pair devices with the [pincode](./pairing.md) for now.

## Where things end up

| Path | Contents |
|---|---|
| `/data/castmill/config/castmill.conf.applied` | Copy of the last applied file (readable by root only) |
| `/data/castmill/config/network/` | The Wi-Fi connection |
| `/data/castmill/config/proxy.env` | Proxy settings for the agent and the players |
| `/data/castmill/config/timesyncd.conf` | NTP servers |

A [factory reset](../operate/factory-reset.md) erases all of it.
