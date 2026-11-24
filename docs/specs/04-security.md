# 04 — Security

Status: v0.3 · Related: SEC-* · Items marked *planned* are not implemented yet ([implementation status](../implementation-status.md)).

## Threat model (summary)

| Threat | Examples | Main mitigations |
|---|---|---|
| Remote attack on the device | Exposed services, browser exploits through content | No inbound ports, sandboxed player, fast Chromium/Electron updates |
| Malicious or tampered update | Compromised mirror or CDN | Signed bundles, key pinning, verified boot |
| Physical access | Stolen device, USB boot, SD card swap | Secure Boot, dm-verity, encrypted `data`, keys in the TPM, boot-order lock |
| Persistence after compromise | Modified system binaries | Read-only verified rootfs; reboot returns to a known state |
| Impersonating a device or the server | Fake player, man-in-the-middle | Mutual TLS with per-device identity; pinned backend CA |
| Supply chain | Vulnerable upstream components | SBOM, CVE scanning, pinned and reproducible builds |

## Chain of trust

**Today:** firmware → GRUB (UEFI) or the Pi firmware → kernel → read-only ext4 rootfs. Updates are verified by RAUC (signed verity bundles, see below); boot itself isn't verified yet. *Planned* (dm-verity in v0.4, Secure Boot in the FC release, see the [roadmap](11-roadmap.md)):

- **x86-64:** UEFI Secure Boot → systemd-boot → signed UKI (kernel + initramfs + cmdline with the dm-verity root hash) → dm-verity rootfs. Ship our own Secure Boot keys, with a documented enrollment process for mini PCs (and a shim-based option where custom key enrollment isn't practical).
- **Raspberry Pi 5 / CM5:** Raspberry Pi signed boot (customer key in OTP) → signed boot image → dm-verity rootfs. Enabling it is **optional and irreversible** per device; document it for customers who need it.
- **Runtime:** dm-verity checks every block read from the rootfs; corruption or tampering is detected and leads to a reboot or rollback.

## Network exposure

- nftables policy: **drop all inbound** except established and related connections. No SSH daemon in production images.
- Outbound only: Castmill backend (WebSocket + HTTPS), CDN for bundles and media, NTP.
- Optional allowlist mode for locked-down customer networks (document the required hosts and ports).
- HTTP(S) proxy support, including a custom CA.

## Identity and secrets

Devices use Castmill's own device identity ([ADR-0008](../adr/0008-reuse-castmill-device-stack.md)):

- Each screen pairs with a pincode in Castmill's device app and receives a **device token**, stored in that output's kiosk-shell profile on `data` (readable only by the `player` user).
- The agent keeps a random **device ID** on `data`; a screen's hardware ID is the device ID plus its output name. A factory reset creates a new device ID, so screens must be paired again.
- Removing a device in Castmill invalidates its token.
- *Planned:* bind the device token to a TPM-held key where a TPM is present (a Castmill platform feature). The original per-device certificate and mutual-TLS design was replaced by ADR-0008.

## Data at rest

- **x86-64 with TPM 2.0:** `data` is LUKS2, with the key sealed to the TPM (bound to PCRs for Secure Boot state).
- **Raspberry Pi:** no TPM. Options are under evaluation (key derived from an OTP secret, or no encryption). See [open questions](01-vision-and-requirements.md#open-questions).

## Least privilege

- `player` user: owns the compositor session and Electron; no capabilities; can only write to the player cache.
- `agent` user: talks to RAUC and logind over D-Bus via polkit rules; may trigger reboots; no general root shell.
- systemd hardening on all services: `ProtectSystem=strict`, `ProtectHome`, `NoNewPrivileges`, `PrivateTmp`, `RestrictAddressFamilies`, `SystemCallFilter`, `CapabilityBoundingSet`.
- Electron: `contextIsolation: true`, `sandbox: true`, no Node.js in content renderers, a strict IPC surface in a preload script, and navigation limited to Castmill origins plus allowed content origins.
- An AppArmor or SELinux profile for the player MAY follow after v1.

## Vulnerability management

- SPDX SBOM generation in every build. Each release publishes the SBOM and a CVE report per image (OE-core's `sbom-cve-check` on the SBOM, against the CVE databases of the release day).
- **Electron/Chromium** is the largest and most exposed component. Track Electron stable releases and ship security releases promptly (NFR-006). CI checks that hardware video decoding still works on every Electron upgrade.
- Kernel: LTS series with regular point-release updates.
- Public security advisory process and `SECURITY.md` with a disclosure contact.
- Prepare for EU **Cyber Resilience Act** obligations: SBOM, vulnerability handling, security updates for the support period, and incident reporting.

## Development vs production

- Development images (SSH, root login) are visibly marked on screen ("DEVELOPMENT IMAGE") and trust only the development key.
- Release images trust only the production root CA; release bundles are re-signed with the production key in a protected, approval-gated job. Production devices reject development bundles and the other way round. See [Release signing](../release-signing.md).
