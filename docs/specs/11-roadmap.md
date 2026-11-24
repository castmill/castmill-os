# 11 — Roadmap

Status: v0.3 · Updated 2026-11-24 · No dates: each release ships when its items are done.

Castmill OS moves to 1.0 in releases. Each one is a GitHub pre-release that can be installed and tested; what's missing on real hardware is tracked in the [implementation status](../implementation-status.md).

## Done

**v0.1 (first test release)**

- [x] Yocto 6.0 "Wrynose" build with kas for `x86-64`, `rpi5` and `vm-arm64`; `meta-castmill` layer; CI (`checks.yml`, `build.yml`, `release.yml`) on standard GitHub runners
- [x] A/B updates with RAUC: GRUB boot counting on UEFI, `castmill-health` commit and rollback, maintenance windows, read-only root filesystem, everything writable on `data`
- [x] Castmill's device stack ([ADR-0008](../adr/0008-reuse-castmill-device-stack.md)): kiosk shell runs Castmill's device app, `castmillOS` bridge, `CastmillOSMachine` in Castmill
- [x] Weston kiosk-shell with one kiosk shell per output ([ADR-0004](../adr/0004-sway-kiosk-compositor.md))
- [x] `castmill.conf` provisioning, factory reset, telemetry, device schedule with display power over HDMI-CEC / DDC/CI
- [x] Development VM for Apple Silicon Macs; documentation site

**v0.2**

- [x] Intel x86-64 USB installer
- [x] Intel GPU rendering (Mesa `iris`) and hardware video decoding (VA-API, Intel media driver)
- [x] Hardware details for Castmill's device page (CPU, memory, GPU, acceleration, decoding, display)
- [x] Raspberry Pi 5 A/B updates with the firmware's tryboot ([ADR-0009](../adr/0009-raspberry-pi-tryboot.md))
- [x] HTML-in-canvas for widgets

## v0.3: managed from Castmill

- [x] OS updates from Castmill ([ADR-0010](../adr/0010-os-updates-from-castmill.md)): release manifest, agent and bridge (Castmill OS); release import, organization policy, device override, update status on the device page (Castmill PR, to merge and deploy)
- [x] Wi-Fi on site: on-screen setup on the status page and `castmill.conf` from a USB stick, both only while offline
- [x] Production signing: offline root CA, signing key in a protected, approval-gated release job ([Release signing](../release-signing.md))
- [x] Layer versions pinned (`kas/*.lock.yml`, updated with `scripts/lock-layers.py`)
- [ ] CVE report published with each release
- [ ] Castmill dashboard: set a device's location; live online status when the dashboard opens; versioned device-app files (no CDN cache delays after a deploy)
- [ ] Release candidates: `v0.3.0-rc.1` (first production-signed release; re-flash test devices), then `rc.2` installed by the VM through Castmill

## v0.4: tamper-evident system

- [ ] dm-verity on the system partitions (initramfs or UKI that sets it up), erofs
- [ ] Fixes from the hardware tests on an Intel mini PC and a Raspberry Pi 5: installer, GPU and video, Pi A/B updates, display power, Wi-Fi on site, OS updates from Castmill
- [ ] RAM, image size and boot time measured against the targets in [spec 01](01-vision-and-requirements.md)
- [ ] Video benchmark on both platforms ([spec 06](06-display-and-media.md))

## Feature complete (FC)

The last pre-1.0 release with new features. To be confirmed:

- [ ] UEFI Secure Boot with signed kernel images (x86-64); disk key in the TPM, encrypted `data`
- [ ] Pi 5 / CM5 signed boot: documentation and tooling (optional per customer)
- [ ] Multi-output: rotation and modes set from Castmill
- [ ] Screenshots and log upload as Castmill features
- [ ] Raspberry Pi Imager entry; bulk provisioning
- [ ] Staged rollouts and channels in Castmill, automatic halt on rollbacks
- [ ] `SECURITY.md`, advisory process, license compliance artifacts in releases (GPL/LGPL sources), trademark policy for the Castmill™ name and logo

## v1.0

Only quality gates after FC, no new features:

- [ ] Automated QEMU tests in CI (`qemu-test.yml`): boot, health check, update, rollback; required for every release. Added after FC, as the last step before 1.0.
- [ ] Provenance attestations for release files
- [ ] Pilot on real screens and a 30-day soak without incidents
- [ ] **v1.0 release**

## After 1.0

- [ ] Native video path (`castmill-videod`) if measurements show it's needed
- [ ] Spanned outputs / video walls
- [ ] Tier 2 hardware: Pi 4, AMD, RK3588
- [ ] Network-boot installation; LTE modems
- [ ] GPS/GNSS location for moving screens ([Castmill integration](../castmill-integration.md#location))
- [ ] Remote shell with auditing; live remote view (WebRTC)
- [ ] Hardware lab with self-hosted runners; power-cut and network chaos tests
- [ ] Production signing key in a cloud KMS/HSM
