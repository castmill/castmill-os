# 09 — Hardware support

Status: Draft v0.1

## Support tiers

| Tier | Meaning |
|---|---|
| **Tier 1** | Built and released for every version, tested in CI on real hardware (hardware-in-the-loop), meets all MUST requirements. |
| **Tier 2** | Built for every version, tested before releases, some MUST requirements may be waived (documented). |
| **Community** | Builds provided, no testing guarantee. |

## Tier 1 (v1.0)

### Intel x86-64 mini PCs

- **Reference SoCs:** Intel N100 / N150 / N200 (Alder Lake-N / Twin Lake), Core i3/i5 Gen 12+.
- **Requirements:** UEFI firmware, Secure Boot capable, TPM 2.0 (firmware TPM is fine), 4 GB RAM (8 GB recommended for 3 outputs), 32 GB+ eMMC/NVMe/SSD.
- **Graphics/video:** Mesa `iris`, VA-API `iHD`; HDMI 2.0 / DisplayPort, 2–3 outputs.
- **Reference devices:** to be chosen (2–3 models from different vendors), with a published hardware compatibility list.

### Raspberry Pi 5 / Compute Module 5

- **Requirements:** 4 GB RAM recommended (2 GB minimum); storage: NVMe (Pi 5 M.2 HAT), eMMC (CM5), or high-endurance SD card.
- **Graphics/video:** Mesa `v3d`; HEVC hardware decoding; 2× micro-HDMI / HDMI, 4K60.
- **Kernel:** Raspberry Pi kernel fork (via `meta-raspberrypi`) until the upstream drivers we need are mainline.
- **Notes:** no TPM; signed boot optional (OTP, irreversible); the official power supply is required (undervoltage is a top field-failure cause, so it's reported in telemetry).

## Development target (not for deployment)

- **`vm-arm64`:** generic arm64 UEFI image for QEMU/UTM on Apple Silicon Macs (HVF acceleration) and arm64 Linux hosts (KVM). Used for functional testing only; it has no hardware video decoding. See the [development guide](../development.md).

## Tier 2 (later)

- Raspberry Pi 4 / CM4
- AMD Ryzen mini PCs (Mesa `radeonsi`, VA-API)
- Rockchip RK3588 (multi-4K/8K decoding; depends on mainline driver maturity)
- GNSS receivers for moving screens: USB receivers (for example u-blox) and LTE modems with GNSS, through `gpsd` or ModemManager ([roadmap](11-roadmap.md), after 1.0)

## Hardware qualification checklist

A device model is added to the compatibility list after passing:

- [ ] boots the generic image (x86) or BSP image
- [ ] Secure Boot with Castmill keys (x86)
- [ ] TPM-sealed `data` unlock (x86)
- [ ] all outputs at native 4K60, hotplug, rotation
- [ ] hardware decoding verified for the platform's profile (see [06](06-display-and-media.md))
- [ ] 72-hour soak with mixed content, zero dropped-frame alarms
- [ ] 100 random power cuts (including during updates) with no failure to boot
- [ ] update + rollback test
- [ ] Wi-Fi and Ethernet stable; CEC and/or DDC/CI works
- [ ] thermal: no sustained throttling at 35 °C ambient in a typical enclosure
