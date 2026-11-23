# Device schedule

Switch screens off outside opening hours. You set the on and off times per screen in the Castmill dashboard (the device's **Schedule**). Castmill OS then:

- **Blanks the screen and stops playback** at the off time. Castmill's device app does this on every platform.
- **Puts the display itself in standby**, and wakes it again at the on time. This is what saves energy: the display uses far more power than the device.

The device itself stays on. It stays reachable from the dashboard, keeps its health checks running, and installs updates in its maintenance window.

## How the display is switched

The agent tries these in order, for each screen:

| Method | Displays | Notes |
|---|---|---|
| **HDMI-CEC** | TVs and most signage displays | Standby at the off time. At the on time, wakes the display and switches it to the device's input. Enable CEC in the display's settings; brands call it Anynet+ (Samsung), SimpLink (LG), Bravia Sync (Sony) or similar. |
| **DDC/CI** | PC monitors and many professional displays | Sets the monitor's power mode to off, and back on. Enable DDC/CI in the monitor's menu if it has the option. |

| Hardware | HDMI-CEC | DDC/CI |
|---|---|---|
| Raspberry Pi 5 / CM5 | Both HDMI ports | Yes |
| Intel mini PCs | Only through USB-C or DisplayPort adapters that pass CEC through; most HDMI ports on Intel PCs have no CEC | Yes, on HDMI and DisplayPort |
| Virtual machine | No | No |

A display that supports neither stays on, showing a black screen during off hours.

::: tip Check it once per display model
Over SSH on a development image, switch a display by hand:

```sh
castmill-agent display HDMI-A-1 off
castmill-agent display HDMI-A-1 on
```

The command prints the method that worked (`cec` or `ddc`), or why neither did.
:::

## Good to know

- **Manual changes are respected.** The schedule switches a display at its on and off times only. If someone switches a display on during off hours, it stays on until the next scheduled change.
- **Offline devices keep their schedule.** The schedule is saved on the device and still applies when the device can't reach Castmill, and after a reboot.
- **The clock must be right.** The schedule waits until the time has been set from NTP or from Castmill. It uses the device's time zone ([`[time]` in `castmill.conf`](../setup/castmill-conf.md#time)).
- **Some TVs disconnect in standby.** Many TVs drop the HDMI connection while in standby. The screen then shows as offline in the dashboard until its on time, and schedule changes made in the meantime reach the device when the screen comes back on. CEC still wakes the TV, and a night-time update is not rolled back just because the screens are in standby.
- **Several screens:** each screen is its own Castmill device with its own schedule.

## Check the state

```sh
castmill-agent status    # "display": power, method and error per screen
journalctl -u castmill-agent | grep schedule
```

| `method` | Meaning |
|---|---|
| `cec` | Switched over HDMI-CEC |
| `ddc` | Switched over DDC/CI |
| `none` | No method worked: the screen is blanked but the display stays on. `error` says why. The agent tries again every 10 minutes, so a display connected later is picked up |
