# 03 — Updates and rollouts

Status: Draft v0.1 · Related: UPD-*, REL-001/002/005, [ADR-0002](../adr/0002-rauc-ab-updates.md), [ADR-0005](../adr/0005-update-rollout-in-castmill-backend.md)

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

## Server side: rollout engine (in Castmill backend)

See [ADR-0005](../adr/0005-update-rollout-in-castmill-backend.md) (status: proposed).

### Artifacts

- RAUC bundles are stored in object storage behind a CDN. RAUC streaming needs HTTP range-request support only.
- Each release has a manifest: `version`, `channel`, `compatible` (hardware family), bundle URL(s), SHA-256, minimum agent version, release notes, `security: bool`.

### Rollout model

- **Channels:** `canary` (internal devices) → `beta` (opt-in customers) → `stable`.
- **Staged rollout** within a channel: 1% → 10% → 50% → 100%, or manual steps.
- **Automatic halt:** pause if the share of devices that rolled back or became unhealthy goes above a threshold (default 2%) in the current stage.
- **Targeting:** organization, device group, hardware family, current version.
- **Pinning:** freeze a device or group on a version (for example during an event).

### Device API (sketch)

> **Revised by [ADR-0008](../adr/0008-reuse-castmill-device-stack.md):** the OS update API is added to the Castmill backend on top of the existing device token and device channel (release registry, desired version, status), not as a separate `/api/os/v1` service. Until then, updates are requested on the device with `castmill-agent update`. See [Castmill integration](../castmill-integration.md#os-updates).

```
GET  /api/os/v1/devices/{id}/desired     → { version, bundle_url, security, activate_policy }
POST /api/os/v1/devices/{id}/status      ← { slot_a, slot_b, booted, state, progress, last_error }
```

Authentication: device mutual-TLS certificate (SEC-005). Alternatively, implement a subset of the Eclipse hawkBit DDI API so the off-the-shelf `rauc-hawkbit-updater` and hawkBit stay possible (see the ADR).

## Signing and keys

- Separate keys for **development** and **production** bundles. Production devices reject development bundles.
- **Production:** an offline root CA (its encrypted key in two maintainers' password managers) approves a signing key valid for 2 years. Release images trust only the root certificate. Bundles are re-signed with the production key in a separate job of `release.yml` that runs in the protected `production-signing` environment after a maintainer's approval, and never runs build code. See [Release signing](../release-signing.md).
- Later: the signing key in a cloud KMS/HSM, so even the signing job never holds it.
- **Rotation:** devices trust the root CA, so a new signing key needs no device update; a new root CA is shipped in a release signed with the current key.

## Open questions

- ~~Exact boot-counting mechanism on Pi 5 (`tryboot` vs U-Boot).~~ Decided: `tryboot` ([ADR-0009](../adr/0009-raspberry-pi-tryboot.md)).
- Adaptive update chunk size vs CDN request limits.
- Whether to also support a USB-stick offline update for air-gapped sites.
