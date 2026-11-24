# ADR-0010: OS updates from Castmill

- Status: Accepted
- Date: 2026-11-24
- Resolves: [ADR-0005](0005-update-rollout-in-castmill-backend.md) (option A, first stage); follows [ADR-0008](0008-reuse-castmill-device-stack.md)

## Context

Devices can install, activate and roll back updates on their own (RAUC, maintenance windows, `castmill-health`), but only after someone runs `castmill-agent update` on the device. Fleets need Castmill to decide which Castmill OS version each device runs and to show how far each device got.

ADR-0008 settled the transport: Castmill's device app, its device token and its device channel, with the `castmillOS` bridge between the device app and `castmill-agent`. What remained open: where releases come from, how an organization chooses a version, and what flows over the channel.

## Decision

### Releases: imported from GitHub

Every Castmill OS release publishes a `manifest.json` asset next to its bundles:

```json
{
  "version": "0.3.0",
  "published_at": "2026-12-01T10:00:00Z",
  "notes_url": "https://github.com/castmill/castmill-os/releases/tag/v0.3.0",
  "security": false,
  "artifacts": {
    "castmill-os-genericx86-64": { "url": "https://github.com/…/castmill-os-bundle-genericx86-64.raucb", "sha256": "…", "size": 393216000 },
    "castmill-os-raspberrypi5": { "url": "…", "sha256": "…", "size": 256901120 }
  }
}
```

`artifacts` is keyed by RAUC `compatible` (one per hardware family). Castmill polls the GitHub releases of the configured repository (default `castmill/castmill-os`), reads new manifests and stores them as OS releases. Pre-releases are imported too and marked as such. Bundles are downloaded by the device straight from the URL in the manifest (GitHub release files support the HTTP range requests RAUC streaming needs).

### Choosing a version: organization policy and device override

| Level | Options |
|---|---|
| Organization | **Latest** (the newest release, pre-releases only if the organization opts in) · **Version X** (stay on X) · **Manual** (only devices with an override update) |
| Device | **Follow organization** (default) · **Version X** · **Update now** (the target version, activated without waiting for the maintenance window) |

Staged rollouts and channels (spec 03) come later and fit on top: they only change how the target version is chosen.

### Device protocol

- **Target, backend → device:** on join and whenever the target changes, the backend pushes `os_update` on the device channel: `{version, url, sha256, size, security, activate: "window" | "now"}` for the device's `compatible` (from its last status report), or `{version: null}` for "no update". A device override also sets `override: true`. The device app hands it to the bridge (`setOsUpdate`), and the agent stores it as its desired version.
- **Status, device → backend:** the device app reads `getOsUpdateStatus` from the bridge and sends `os_update:report` when it changes: `compatible`, `current_version`, `target_version`, `state` (`idle`, `installing` with `progress`, `installed`, `activating`, `running`, `rolled_back`, `failed`, or `unsupported` without RAUC), the `version` that state refers to, and `last_error`. The backend keeps the last report on the device and shows it in the dashboard.
- **Several outputs:** each output is its own Castmill device but they share one OS. The agent applies the most recent target it received and every output reports the same status. An organization normally gives all outputs of a device the same target; if they differ, the most recent instruction wins.

### Safety on the device

- RAUC only installs bundles signed with a key in the device's keyring, so the backend can only offer releases signed for these devices.
- The agent refuses to install an older version than the one running unless the target is a device override (an explicit downgrade).
- A version that rolled back on this device isn't tried again (unchanged).
- Activation still happens inside the maintenance window, except for "Update now" and security releases (spec 03).

## Consequences

- `release.yml` writes `manifest.json` (with SHA-256 and size per bundle).
- Agent: `set_os_update` and `os_update_status` on the player socket, with the downgrade check.
- Kiosk shell: bridge methods `setOsUpdate` and `getOsUpdateStatus`.
- Castmill (PR): OS release table and importer, organization and device settings, `os_update` / `os_update:report` on the device channel, an OS section on the device page and in the organization settings, and `CastmillOSMachine` support.
- `castmill-agent update` stays for development and for devices without Castmill.
