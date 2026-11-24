# ADR-0005: Update rollout engine in the Castmill backend

- Status: Accepted (option A; first stage in [ADR-0010](0010-os-updates-from-castmill.md))
- Date: 2026-11-22

## Context

Devices need to know which OS version to install and when. Fleet rollouts need channels, staged percentages, automatic halts and reporting. Devices, organizations, groups, authentication and schedules already live in the Castmill backend.

On the device, RAUC does the heavy work: verification, slot writing, streaming/adaptive downloads, rollback. Bundles can be served as static files from object storage and a CDN (RAUC streaming needs only HTTP range requests). What remains is the decision of what each device should run and when.

## Options

### A. Native rollout engine in Castmill (preferred)

- Release manifests + rollout state machine (channels, stages, halt thresholds, pins, maintenance windows) in the Castmill backend.
- Device API served by the backend; the agent drives RAUC over D-Bus.
- **Pros:** one device registry, one authentication system (device mTLS), one UI; maintenance windows and groups reused; no extra service to run.
- **Cons:** we build and maintain rollout logic ourselves.

### B. Eclipse hawkBit as a separate service

- `rauc-hawkbit-updater` on the device, hawkBit server next to Castmill.
- **Pros:** mature rollout management, existing device client in `meta-rauc`.
- **Cons:** extra Java/Spring service to run and secure; a second device registry and authentication to keep in sync; a second UI; streaming can hit hawkBit's default range-request limits.

### C. Hybrid: hawkBit-compatible API in Castmill

- Implement the subset of hawkBit's DDI API that the device needs inside the Castmill backend.
- Keeps the option of using `rauc-hawkbit-updater` or switching to real hawkBit later, at the cost of following an external API's design.

## Proposed decision

**Option A**, designing the device API so that a move to C remains cheap. Revisit if Castmill OS needs to be usable without the Castmill backend (other CMSs, OEMs). In that case B or C becomes more attractive.

## Consequences

- Backend work: release registry, rollout engine, device OS status views (see spec 03 and 07).
- Bundle hosting on object storage + CDN with range-request support.
