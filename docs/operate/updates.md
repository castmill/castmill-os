# Updates and rollback

Castmill OS updates the whole system at once and can always go back. An update never leaves a device half-updated or unbootable.

::: info Platforms
A/B updates work on **Intel x86-64** devices and the **virtual machine**. On the Raspberry Pi 5 they come later; until then, flash the new image.
:::

## How an update works

```text
 running slot A                      slot B (inactive)
 ─────────────                       ─────────────────
 1. download + verify signature  →   2. written in the background
                                        (content keeps playing)
 3. maintenance window: reboot   →   4. trial boot of slot B
                                     5. castmill-health checks the device
                                        ├─ healthy  → slot B committed
                                        └─ not      → back to slot A
```

1. **Download.** The agent streams a signed RAUC bundle over HTTPS. Bundles signed with other keys are rejected.
2. **Install.** RAUC writes the new system to the inactive slot while the screens keep playing. A power cut at this point does no harm: the running slot isn't touched and the new one isn't activated yet.
3. **Activate.** The device reboots into the new slot inside its **maintenance window**: 03:00–05:00 device-local time by default.
4. **Trial boot.** GRUB boots the new slot with a counter of three attempts. If it doesn't come up, GRUB goes back to the previous slot.
5. **Health check.** Within 10 minutes `castmill-health` must see the system services running, `data` mounted, every connected screen rendering frames and Castmill reachable. Then it commits the new slot. Otherwise it marks the slot bad and the device goes back to the previous version.

A version that was rolled back isn't installed again. A network outage during the trial boot never causes a rollback, so a broken network at the site can't undo a good update.

## Who starts an update

**From Castmill (planned).** Castmill will keep a registry of Castmill OS releases and let you choose the OS version for each device or group, with channels and staged rollouts. The device then reports the update status in the dashboard. This needs Castmill backend support, which is the next step for the integration. See [Castmill integration](../castmill-integration.md#os-updates).

**On the device (now).** On development images, or with support access, an update can be requested locally:

```sh
castmill-agent update https://example.com/castmill-os-bundle-genericx86-64.raucb 0.2.0
castmill-agent update https://example.com/castmill-os-bundle-genericx86-64.raucb 0.2.0 --now
castmill-agent status
```

Without `--now`, the update is installed right away and activated in the next maintenance window. With `--now`, the device reboots into it as soon as it's installed.

## Update states

`castmill-agent status` shows the state of the last update:

| State | Meaning |
|---|---|
| `idle` | No update in progress |
| `installing` | Downloading and writing the inactive slot |
| `installed` | Written; waiting for the maintenance window |
| `activating` | Rebooting into the new slot, on trial |
| `running` | The new version passed its health check and is committed |
| `rolled_back` | The new version failed; the device went back. The reason is in `last_error`. |
| `failed` | The download or the installation failed; the running system is unchanged |

## Try it

The [development guide](../development.md#test-an-update-and-a-rollback) explains how to test an update and a rollback in the virtual machine.
