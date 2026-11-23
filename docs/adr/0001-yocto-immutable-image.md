# ADR-0001: Yocto-built immutable OS image

- Status: Proposed
- Date: 2026-11-22

## Context

Signage devices must be identical, unattended and safe to update. Package-based distributions (Debian, Arch, Ubuntu) drift over time, update packages one by one (a failure can leave a half-updated system), and ship far more than a player needs.

## Decision

Build Castmill OS with the **Yocto Project 6.0 "Wrynose" LTS** (supported until April 2030), configured with kas, as complete read-only images (erofs/squashfs + dm-verity). There is no package manager on the device. One image per hardware family.

## Alternatives considered

- **Buildroot:** simpler and faster to learn, but weaker SBOM/CVE tooling and fewer maintained BSP and security layers. A good fallback if Yocto's complexity becomes a problem.
- **Debian-based image builder (mkosi, debos) + A/B:** familiar packages, but larger images and less control over the build.
- **Existing immutable OSes (Ubuntu Core, balenaOS, Fedora IoT/bootc):** less work up front, but they bring vendor ecosystems, larger footprints, or dependence on someone else's update infrastructure and release schedule.

## Consequences

- Steeper learning curve and longer clean builds (shared-state caching is needed).
- Full control over contents, size, security hardening and SBOM.
- Since Yocto 6.0 there's no combined "poky" repository; kas pulls bitbake, OE-core and meta-yocto separately.
- Board support: `meta-raspberrypi` (Wrynose branch available) and OE-core/meta-yocto generic machines for x86-64 and the arm64 dev VM. The Intel VA-API driver (`intel-media-driver`, with `gmmlib`) comes from `meta-intel` (Wrynose branch on git.yoctoproject.org), used on x86-64 only and with the generic machine.
- the Electron runtime has to be integrated as a recipe (packaging the prebuilt Electron release vs building Chromium is a Phase 0 task).
