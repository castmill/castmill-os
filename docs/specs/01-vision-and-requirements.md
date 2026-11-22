# 01 — Vision, scope and requirements

Status: Draft v0.1 · Owner: TBD

## Vision

Castmill OS is a slim, immutable, secure Linux distribution that turns supported hardware into a reliable Castmill signage player. Once installed and enrolled, a device should run unattended for years. It updates itself, recovers from failures without help, and never needs someone on site except for hardware faults.

## Goals

- A purpose-built OS for the Castmill player on Intel x86-64 and Raspberry Pi hardware.
- Safe, automatic, atomic updates that can be rolled back, with security patches delivered quickly.
- Hardware-accelerated graphics and video, 4K output and multiple displays.
- Installation in minutes and zero-touch enrollment for large rollouts.
- Small disk and memory footprint.
- Fully open source and reproducible.

## Non-goals

- A general-purpose desktop or server distribution.
- A package manager on the device. Software changes only through whole-image updates.
- Running arbitrary third-party apps. (A sandboxed extension mechanism may come later; it is out of scope for v1.)
- Supporting every ARM board. Hardware is added deliberately, by tiers.

## Requirement levels

**MUST** = required for v1.0 · **SHOULD** = expected unless there's a documented reason · **MAY** = optional.
IDs are stable and referenced from the other specs and tests.

## Functional requirements

### Security (see [04](04-security.md))

| ID | Requirement | Level |
|---|---|---|
| SEC-001 | The root filesystem is read-only and integrity-verified (dm-verity) at runtime. | MUST |
| SEC-002 | Update bundles are cryptographically signed and verified before installation. | MUST |
| SEC-003 | Verified boot chain on hardware that supports it (UEFI Secure Boot on x86-64, signed boot on Pi 5/CM5). | MUST (x86) / SHOULD (Pi) |
| SEC-004 | No inbound network ports open by default. | MUST |
| SEC-005 | Each device has a unique cryptographic identity used for mutual TLS with the Castmill backend. | MUST |
| SEC-006 | Private keys are stored in a TPM when the hardware has one. | SHOULD |
| SEC-007 | The data partition is encrypted at rest when a TPM is available. | SHOULD |
| SEC-008 | Security updates are downloaded and staged automatically without operator action. | MUST |
| SEC-009 | Each release ships an SBOM (SPDX) and a CVE report. | MUST |
| SEC-010 | The player and agent run as unprivileged users with systemd sandboxing. | MUST |

### Reliability (see [05](05-reliability.md))

| ID | Requirement | Level |
|---|---|---|
| REL-001 | A failed update is rolled back automatically, without human intervention. | MUST |
| REL-002 | A new slot is committed only after a health check passes (network, player running, frame rendered). | MUST |
| REL-003 | A hardware watchdog is enabled and fed by systemd. | MUST |
| REL-004 | Crashed or hung player, compositor or agent is detected and restarted. | MUST |
| REL-005 | Power loss at any moment, including during an update, never leaves the device unbootable. | MUST |
| REL-006 | Writes to storage are minimised to extend SD/eMMC life (volatile logs, no swap on flash). | MUST |
| REL-007 | Factory reset restores a working, unenrolled device by wiping the data partition. | MUST |
| REL-008 | The device recovers on its own from network loss, display hotplug and time drift. | MUST |

### Updates (see [03](03-updates.md))

| ID | Requirement | Level |
|---|---|---|
| UPD-001 | Updates are A/B and atomic. | MUST |
| UPD-002 | Downloading and writing an update never disturbs playback. | MUST |
| UPD-003 | Reboots into a new version happen only inside a configurable maintenance window, unless an operator forces it. | MUST |
| UPD-004 | Release channels (e.g. canary, beta, stable) and staged percentage rollouts are supported. | MUST |
| UPD-005 | A rollout pauses automatically when the failure rate goes above a threshold. | SHOULD |
| UPD-006 | Updates are streamed and only changed chunks are downloaded where possible. | SHOULD |

### Display and media (see [06](06-display-and-media.md))

| ID | Requirement | Level |
|---|---|---|
| MED-001 | GPU-accelerated rendering through KMS/DRM and Mesa (no X11). | MUST |
| MED-002 | Hardware video decoding on every tier-1 platform for the codecs Castmill delivers to that platform. | MUST |
| MED-003 | 4K60 output and 4K video playback with no dropped frames on tier-1 Intel hardware. | MUST |
| MED-004 | 4K video playback on Raspberry Pi 5 (HEVC). | SHOULD |
| MED-005 | Multiple outputs with independent content, plus per-output resolution and rotation. | MUST |
| MED-006 | Display power control on a schedule (HDMI-CEC and/or DDC/CI). | SHOULD |
| MED-007 | The device reports its decode capabilities so the backend can pick suitable renditions. | MUST |

### Device management (see [07](07-device-management.md))

| ID | Requirement | Level |
|---|---|---|
| MGT-001 | Remote reboot, screenshot, log collection and status reporting from Castmill. | MUST |
| MGT-002 | An on-demand, time-limited, audited remote shell, opened only by an authorized operator. | SHOULD |
| MGT-003 | Remote access keeps working when the player has crashed. | MUST |
| MGT-004 | Health and telemetry: CPU, memory, temperature, storage wear, dropped video frames. | SHOULD |

### Installation and provisioning (see [08](08-installation-and-provisioning.md))

| ID | Requirement | Level |
|---|---|---|
| PRV-001 | x86-64 unattended install from USB to internal storage in under 3 minutes. | MUST |
| PRV-002 | Pi images can be flashed with standard tools (Raspberry Pi Imager, bmaptool, Etcher). | MUST |
| PRV-003 | Zero-touch enrollment: pairing code or QR code on screen, or a pre-provisioning file for bulk enrollment. | MUST |
| PRV-004 | Network-boot (PXE/HTTP boot) installation for large rollouts. | MAY |
| PRV-005 | Wi-Fi and proxy configuration without a keyboard. | MUST |

### Development (see [development guide](../development.md))

| ID | Requirement | Level |
|---|---|---|
| DEV-001 | Developers can run Castmill OS on an Apple Silicon Mac in a VM (QEMU/UTM with hardware virtualization), booting the same way as real hardware. | MUST |
| DEV-002 | The development VM supports multiple virtual displays, A/B updates and rollback, so these can be tested before shipping. | MUST |
| DEV-003 | Images for every target are built in CI for every branch and can be downloaded with one command. | MUST |
| DEV-004 | Development images (SSH, root login, debug tools) are clearly separated from production images and never production-signed. | MUST |

### Licensing and compliance (see [ADR-0007](../adr/0007-licensing.md))

| ID | Requirement | Level |
|---|---|---|
| LIC-001 | Every file in the repository has machine-readable licensing (REUSE compliant). | MUST |
| LIC-002 | Production images contain no GPL-3.0, LGPL-3.0 or AGPL-3.0 packages, unless an ADR approves an exception. | MUST |
| LIC-003 | Each release publishes the license manifest, SPDX SBOM and the corresponding source of all GPL/LGPL packages. | MUST |

## Non-functional targets

These are initial targets. The prototype phase will validate them.

| ID | Target |
|---|---|
| NFR-001 | Compressed image ≤ 350 MB (including Electron). |
| NFR-002 | Idle memory use of the OS plus agent, excluding the player, ≤ 150 MB. |
| NFR-003 | Time from power-on to first content ≤ 30 s on tier-1 hardware. |
| NFR-004 | Minimum supported RAM 2 GB; recommended 4 GB. |
| NFR-005 | 30-day soak test with zero manual interventions per release candidate. |
| NFR-006 | Critical CVE fixes released within 7 days of upstream availability. |

## Open questions

1. ~~**License**~~ Decided: permissive licenses, GPLv3-free production images ([ADR-0007](../adr/0007-licensing.md)).
2. **Standalone use:** will Castmill OS be usable without the Castmill backend, for example with other CMSs or by OEMs? This affects the update server choice ([ADR-0005](../adr/0005-update-rollout-in-castmill-backend.md)).
3. **Pi encryption:** is at-rest encryption on Raspberry Pi (no TPM) required, or is CM5 with eMMC the recommended secure Pi option?
4. **Player delivery:** does the player ship only inside the OS image, or can its web assets update independently? (Current proposal: inside the image. See [02](02-system-architecture.md).)
5. **Branding and naming** of images and channels, and whether OEM/white-label builds are needed (own logo, backend URL and name). For now the boot logo, backend URL and player URL are build-time variables in `meta-castmill`, so a white-label build would be a configuration change, not a fork.
