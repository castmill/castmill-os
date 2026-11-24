# Architecture Decision Records

Short documents that record significant decisions, their context and their consequences.

## Process

1. Copy the template below into `NNNN-short-title.md` (next free number).
2. Open a pull request with status **Proposed**.
3. After discussion, set the status to **Accepted**, **Rejected**, or later **Superseded by ADR-XXXX**.

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-yocto-immutable-image.md) | Yocto-built immutable OS image | Proposed |
| [0002](0002-rauc-ab-updates.md) | RAUC for A/B updates | Proposed |
| [0003](0003-electron-player-runtime.md) | Electron as the player runtime for v1 | Accepted (short term) |
| [0004](0004-sway-kiosk-compositor.md) | Kiosk compositor for v1 | Accepted: Weston kiosk-shell |
| [0005](0005-update-rollout-in-castmill-backend.md) | Update rollout engine in the Castmill backend | Accepted (option A) |
| [0006](0006-github-actions-ci.md) | GitHub and GitHub Actions for CI/CD | Accepted |
| [0007](0007-licensing.md) | Licensing (MIT / Apache-2.0 / CC BY 4.0, GPLv3-free images) | Accepted |
| [0008](0008-reuse-castmill-device-stack.md) | Reuse Castmill's device stack | Accepted |
| [0009](0009-raspberry-pi-tryboot.md) | A/B boot on Raspberry Pi 5 with the firmware's tryboot | Accepted |
| [0010](0010-os-updates-from-castmill.md) | OS updates from Castmill | Accepted |

## Template

```markdown
# ADR-NNNN: Title

- Status: Proposed | Accepted | Rejected | Superseded by ADR-XXXX
- Date: YYYY-MM-DD

## Context
## Decision
## Alternatives considered
## Consequences
```
