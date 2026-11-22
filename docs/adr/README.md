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
| [0004](0004-sway-kiosk-compositor.md) | Kiosk compositor for v1 (Sway or Weston kiosk-shell) | Proposed (open; v0.1 implements Weston) |
| [0005](0005-update-rollout-in-castmill-backend.md) | Update rollout engine in the Castmill backend | Proposed |
| [0006](0006-github-actions-ci.md) | GitHub and GitHub Actions for CI/CD | Accepted |
| [0007](0007-licensing.md) | Licensing (MIT / Apache-2.0 / CC BY 4.0, GPLv3-free images) | Accepted |

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
