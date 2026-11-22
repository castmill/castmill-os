# ADR-0002: RAUC for A/B updates

- Status: Proposed
- Date: 2026-11-22

## Context

Updates must be atomic, signed, safe during power loss, roll back automatically and be efficient on slow networks.

## Decision

Use **RAUC** with symmetric A/B rootfs slots, X.509-signed bundles (verity format), HTTP streaming and adaptive updates. The bootloader does the boot counting. `castmill-health` commits the slot (`mark-good`) or rejects it (`mark-bad`).

## Alternatives considered

- **Mender:** most complete out of the box, including a server, but the open-source edition gates some fleet features and it pushes towards its own server.
- **SWUpdate:** very flexible, more configuration work.
- **OSTree:** efficient file-level deltas, but a different model (no full-image verity per slot) and more complex boot integration on Pi.

## Consequences

- Lightweight client (LGPL-2.1); good Yocto integration through `meta-rauc` and community BSP examples.
- Server side is our responsibility (see ADR-0005), with `rauc-hawkbit-updater` + Eclipse hawkBit available as an off-the-shelf fallback.
- Full slots double the rootfs space needed on disk.
