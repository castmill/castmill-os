# Troubleshooting

## Where to look

| Where | What you get |
|---|---|
| **Console banner** | At boot: the Castmill OS version, the machine and the active slot. Include it in bug reports. |
| **Status page** | Shown on a screen while Castmill's device app can't be loaded: device name, output, version and the connection error. |
| **Castmill dashboard** | For paired screens: online state, device info, telemetry. |
| **`castmill-agent status`** | Development images, over SSH: the agent's view of the device, as JSON. |

On development images, connect with `ssh root@<device-ip>` (or `ssh -p 2222 root@localhost` for the VM). Production images have no SSH or console login.

```sh
castmill-agent status                       # outputs, backend, clock, update state
journalctl -b -u castmill-agent             # the agent
journalctl -b -u castmill-provision         # castmill.conf
journalctl -b -u castmill-health            # trial boots and rollbacks
journalctl -b -u castmill-session           # the display session (Weston)
journalctl -b -u 'castmill-player@*'        # the kiosk shells, one per output
```

## Common problems

### The screens stay black

- Check that the screen was connected and on before the device booted. Screens plugged in later should start on hotplug; if one doesn't, reboot.
- Raspberry Pi: use the official power supply. Undervoltage makes the Pi throttle or reset.
- If the console banner shows and then nothing else, the display session isn't starting: check `castmill-session` in the journal.

### The status page says Castmill isn't reachable

- **Ethernet:** check the cable and that the network gives out addresses with DHCP.
- **Wi-Fi:** check the `ssid` and `psk` in [`castmill.conf`](../setup/castmill-conf.md). A file with an error stays on the boot partition and isn't applied.
- **Firewalls and proxies:** the device needs outgoing HTTPS (port 443) to the Castmill server. Behind a proxy, set [`[network.proxy]`](../setup/castmill-conf.md#proxy); behind a TLS-inspecting proxy, add its CA certificate.
- The device retries every 30 seconds, so it recovers by itself once the network is fixed.

### `castmill.conf` wasn't applied

- It must be called exactly `castmill.conf` and sit in the root of the **boot** partition.
- If it's still on the boot partition after a boot, it has an error. Check `journalctl -u castmill-provision` on a development image, or compare it with the [reference](../setup/castmill-conf.md#reference): unknown keys are rejected.

### The time is wrong

The device needs the right time for HTTPS and schedules. It uses NTP and, when NTP is blocked, the time from Castmill's HTTPS responses. Set custom NTP servers and the time zone in [`[time]`](../setup/castmill-conf.md#time).

### A widget shows the wrong city

Weather and other location widgets show **London, United Kingdom** until you pick a location in the widget's settings in the Castmill dashboard. The device doesn't report where it is; see [Location](../castmill-integration.md#location).

### An update was rolled back

`castmill-agent status` shows `rolled_back` with the reason in `last_error`, and the device keeps running the previous version. See [Updates and rollback](./updates.md).

### Start over

A [factory reset](./factory-reset.md) erases the pairing and all settings and starts the device as new.

## Reporting a problem

Open an [issue on GitHub](https://github.com/castmill/castmill-os/issues) with:

- the version from the console banner, and the hardware
- what you did and what happened
- on development images, the output of `castmill-agent status` and the relevant `journalctl` lines
