# 11 — Roadmap

Status: Draft v0.1 · Dates are indicative.

## Phase 0: Prototype (≈ 4 weeks)

Goal: lower the main technical risks before committing to the full architecture.

- [x] kas configuration for `x86-64`, `rpi5`, `vm-arm64` (Yocto 6.0 Wrynose) + minimal `meta-castmill` layer
- [x] `checks.yml`, `build.yml`, CODEOWNERS, Dependabot (drafts)
- [x] Initial implementation (v0.1): A/B layout with GRUB boot counting, RAUC config and bundle, read-only rootfs with `/data`, `castmill-agent` + `castmill-health` + provisioning, Weston kiosk session with the Electron kiosk shell. See [implementation status](../implementation-status.md).
- [ ] GitHub repository with branch rulesets; larger-runner group `castmill-os-builders`; Actions cache limit raised
- [ ] First green builds of all three targets; measure clean and cached build times (6-hour job limit)
- [ ] Development VM booting on Apple Silicon Mac (`scripts/run-vm.sh`)
- [ ] Pin layers with `kas lock`
- [ ] Compositor decision (ADR-0004) and Intel VA-API driver source
- [ ] Check impact of GPLv3-free production images (`INCOMPATIBLE_LICENSE`, ADR-0007)
- [ ] Sway kiosk + Electron Castmill player booting to content on both
- [ ] Video benchmark matrix (see [06](06-display-and-media.md)), comparing the Yocto image against Raspberry Pi OS with the same player and test videos:
  - Intel: VA-API 4K H.264/HEVC/VP9/AV1
  - Pi 5: H.264 1080p/4K, HEVC 4K; confirm the decoder actually used
- [ ] Quick test: GStreamer HEVC 4K into a Wayland surface below a transparent Electron window on Pi 5
- [ ] RAUC A/B with boot counting on both platforms; decide Pi 5 `tryboot` vs U-Boot
- [ ] RAM, image size and boot-time measurements against the NFR targets
- [ ] **Output:** updated specs + accepted ADRs

## Phase 1: MVP (≈ 3 months)

- [ ] Production images for x86-64 and Pi 5; read-only rootfs + dm-verity
- [ ] RAUC A/B with `castmill-health` commit and rollback
- [x] Reuse Castmill's device stack ([ADR-0008](../adr/0008-reuse-castmill-device-stack.md)): kiosk shell runs Castmill's device app; `castmillOS` bridge; `CastmillOSMachine` in Castmill
- [ ] OS updates in the Castmill backend: release registry, desired version, channels and staged rollouts, update status
- [ ] `castmill-agent` v1: updates, maintenance windows, telemetry, health (done in v0.1); screenshots and log upload as Castmill features
- [ ] Multi-output (independent content), rotation
- [ ] Per-device content profiles in the Castmill transcoder
- [ ] Rollout engine v1 in the Castmill backend: channels, manual staged rollouts, status
- [ ] x86 USB installer, Pi images + Imager entry, `castmill.conf` bulk provisioning
- [ ] `build.yml` + `qemu-test.yml` + `nightly.yml`; first hardware lab with self-hosted lab runners
- [ ] `release.yml` with protected signing environment, GitHub Releases, provenance attestations and Castmill manifest registration
- [ ] Pilot with internal and friendly-customer screens

## Phase 2: Hardening (≈ 3 months)

- [ ] UEFI Secure Boot with signed UKIs; TPM-sealed encrypted `data`; TPM-backed identity
- [ ] Pi 5/CM5 signed boot (optional) documentation and tooling
- [ ] Automatic rollout halt; security release fast lane
- [ ] Remote shell with auditing
- [ ] CEC/DDC display power schedules
- [ ] Power-cut and network chaos suites in the lab; 30-day soak gate
- [ ] SBOM/CVE publication, `SECURITY.md`, advisory process
- [ ] License compliance artifacts in releases (manifest + GPL/LGPL sources); trademark policy for the Castmill OS name and logo
- [ ] **v1.0 release**

## Phase 3: Expansion

- [ ] Native video path (`castmill-videod`) if Phase 0/1 data shows it's needed
- [ ] Spanned outputs / video walls
- [ ] Tier 2 hardware: Pi 4, AMD, RK3588
- [ ] Network-boot installation; LTE modems
- [ ] GPS/GNSS location for moving screens (USB receivers, LTE modems with GNSS): reported by the agent, answered by `getLocation`, opt-in per organisation. Fixed screens get their location from the Castmill dashboard ([Castmill integration](../castmill-integration.md#location)).
- [ ] Live remote view (WebRTC)
