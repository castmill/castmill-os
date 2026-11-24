# 03 — Updates and rollouts

Status: v0.3 · Related: UPD-*, REL-001/002/005, [ADR-0002](../adr/0002-rauc-ab-updates.md), [ADR-0005](../adr/0005-update-rollout-in-castmill-backend.md)

## Principles

1. **Whole-image, A/B, atomic.** The running slot is never modified.
2. **Install anytime, reboot only in a window.** Downloading and writing the inactive slot happen in the background at any time. Activation (reboot) happens only inside the device's maintenance window.
3. **Commit only when healthy.** A new slot becomes permanent only after the health check passes. Otherwise the device rolls back automatically.
4. **The server decides "what and when"; the device decides "is it safe".**

## Device side

### Components

- **RAUC:** bundle verification (X.509 signatures), slot writing, slot status, HTTP streaming and adaptive updates.
- **castmill-agent:** polls or receives update instructions from Castmill, calls RAUC over D-Bus, enforces the maintenance window, reports progress.
- **Bootloader boot counting:** for rollback when the new slot fails to boot or fails the health check.

### Update flow

```
 Backend                        Agent                         RAUC / bootloader
   │  desired version + URL       │                                │
   │ ───────────────────────────► │                                │
   │                              │ check: compatible, free space, │
   │                              │ not pinned/frozen              │
   │                              │ ── install(url) ─────────────► │ verify signature
   │                              │                                │ stream + write inactive slot
   │ ◄── progress / installed ─── │ ◄── completed ──────────────── │ mark slot "pending"
   │                              │ wait for maintenance window    │
   │                              │ ── reboot ───────────────────► │ boot new slot (tries = 3)
   │                              │                                │
   │                              │ castmill-health passes?        │
   │                              │   yes → mark-good              │ commit
   │ ◄── running vX (ok) ──────── │   no  → mark-bad + reboot      │ fall back to old slot
   │ ◄── rolled back (reason) ─── │                                │
```

### Maintenance windows

- Configured per device or device group in Castmill (for example 03:00–05:00 local time). Stored on the device so they still apply when offline.
- The device needs a valid time (see [05](05-reliability.md)). With no valid time, activation is postponed.
- An operator can force **"apply now"**.
- **Security-critical** releases MAY use a shorter policy (for example the next window, or within 24 hours, even outside the window), configurable per organization.

### Update types

| Type | Delivery | Activation |
|---|---|---|
| OS + player release | RAUC bundle (adaptive/streamed) | Reboot in maintenance window |
| Security hotfix | RAUC bundle | Per security policy |
| Configuration (network, display, windows) | Agent message | Immediate or at next player restart; no reboot |

Partial package updates and live patching are explicitly out of scope.

## Server side: Castmill decides the version

Implemented as the first stage of [ADR-0005](../adr/0005-update-rollout-in-castmill-backend.md) (option A), designed in [ADR-0010](../adr/0010-os-updates-from-castmill.md):

### Artifacts

- Each release publishes its RAUC bundles and a `manifest.json` (version, pre-release flag, release notes URL, security flag and, per hardware family `compatible`, the bundle URL, SHA-256 and size) as GitHub release files. GitHub serves the HTTP range requests RAUC streaming needs.
- Castmill imports new manifests from the GitHub releases every hour. A release GitHub marks as pre-release stays a pre-release.

### Choosing the version (today)

- **Organization policy:** manual (default), the latest release (pre-releases only if the organization opts in), or a fixed version.
- **Device override:** follow the organization, or a specific version, optionally activated right away ("update now").
- Castmill sends each device its target over the device channel (`os_update`); the device reports its progress (`os_update:report`), shown on the device page. Details: [Castmill integration](../castmill-integration.md#os-updates).

### Rollout model (planned)

- **Channels:** `canary` (internal devices) → `beta` (opt-in customers) → `stable`.
- **Staged rollout** within a channel: 1% → 10% → 50% → 100%, or manual steps.
- **Automatic halt:** pause if the share of devices that rolled back or became unhealthy goes above a threshold (default 2%) in the current stage.
- **Targeting:** organization, device group, hardware family, current version.

They only change how the target version is chosen; the device protocol stays the same.

## Signing and keys

- Separate keys for **development** and **production** bundles. Production devices reject development bundles.
- **Production:** an offline root CA (its encrypted key in two maintainers' password managers) approves a signing key valid for 2 years. Release images trust only the root certificate. Bundles are re-signed with the production key in a separate job of `release.yml` that runs in the protected `production-signing` environment after a maintainer's approval, and never runs build code. See [Release signing](../release-signing.md).
- Later: the signing key in a cloud KMS/HSM, so even the signing job never holds it.
- **Rotation:** devices trust the root CA, so a new signing key needs no device update; a new root CA is shipped in a release signed with the current key.

## Open questions

- ~~Exact boot-counting mechanism on Pi 5 (`tryboot` vs U-Boot).~~ Decided: `tryboot` ([ADR-0009](../adr/0009-raspberry-pi-tryboot.md)).
- Adaptive update chunk size vs CDN request limits.
- Whether to also support a USB-stick offline update for air-gapped sites.
