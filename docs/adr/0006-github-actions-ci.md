# ADR-0006: GitHub and GitHub Actions for CI/CD

- Status: Accepted
- Date: 2026-11-22

## Context

Castmill OS is open source and will be developed in public. Every change must be validated, and every release built, tested, signed and published automatically. Yocto builds are heavy (large disk, many CPU hours) and the project needs real hardware for testing. Our organization is on **GitHub Enterprise Cloud**.

## Decision

- Host the repository on **GitHub**. Use **GitHub Actions** for all CI/CD: checks, builds, QEMU tests, hardware-lab tests, signing and publishing.
- **Everything runs on GitHub-hosted runners:**
  - standard runners for checks (including fork PRs);
  - **larger runners** (Enterprise) for Yocto builds and QEMU tests;
  - a larger runner with a **static IP** that drives the hardware lab remotely through a lab controller, so no self-hosted runners are needed;
  - a protected environment, with required reviewers, for signing.
- Build caches (sstate, downloads) live in the **GitHub Actions cache**, with the repository limit raised above 10 GB. Only trusted pushes write to it.
- Publish images, SBOMs and checksums as **GitHub Releases**; serve RAUC bundles from object storage/CDN, managed by the Castmill backend.

## Alternatives considered

- **Self-hosted ephemeral build VMs:** cheaper per build hour, but infrastructure to operate and secure. GitHub advises against self-hosted runners on public repositories.
- **GitLab CI / Buildkite / Jenkins:** good options for heavy builds, but a separate system from where the code and community are.
- **Standard GitHub-hosted runners only:** not viable for Yocto (14 GB disk).

## Consequences

- Larger-runner minutes and extra cache storage are paid, also for a public repository. Fork PR builds are started by maintainers, which keeps costs under control.
- The 6-hour job limit still applies. The sstate cache keeps normal builds well under it. A clean build of everything must also fit, so it's measured in Phase 0 (and Chromium/Electron is packaged from official binaries rather than built from source, unless that changes).
- Strict public-repository rules (spec 10): cache writes only from trusted builds, actions pinned to a commit SHA, OIDC instead of stored cloud keys, protected signing environment, lab reachable only from our static IPs.
- Release artifacts get build provenance attestations, so users can verify where an image came from.
