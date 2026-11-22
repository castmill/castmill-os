# 10 — Build, release and testing

Status: Draft v0.3 · Related: [ADR-0006](../adr/0006-github-actions-ci.md)

The repository is hosted on **GitHub** (Enterprise Cloud organization). **GitHub Actions** automates testing, validation, building and releasing every version. All builds run on GitHub-hosted runners.

## Build

- Yocto 6.0 "Wrynose" LTS, configured with **kas**: `kas/x86-64.yml`, `kas/rpi5.yml`, `kas/vm-arm64.yml` (development VM), plus the `kas/dev.yml` overlay. The same kas files are used locally and in CI. Builds run in the official kas container (`kas-container`), so the host needs only Docker.
- The **shared state (sstate) cache and downloads** are kept in the **GitHub Actions cache**, with the repository's cache limit raised above the free 10 GB (paid per GB). Without a cache, Yocto builds don't fit the CI time limit. If cache entries turn out to be too large or slow, move sstate to object storage (decide in Phase 0).
- Reproducible builds enabled; build outputs are content-addressed.
- Build outputs per target: disk image + bmap, RAUC bundle, installer (x86), SBOM (SPDX), CVE report, license manifest, source archive of GPL/LGPL packages (Yocto `archiver` class).
- Production builds set `INCOMPATIBLE_LICENSE` to exclude GPL-3.0/LGPL-3.0/AGPL-3.0 packages (ADR-0007); development images may include them.

## Versioning

- OS version: [Semantic Versioning 2.0.0](https://semver.org/) `MAJOR.MINOR.PATCH`, starting at `0.1.0`. Git tags `vX.Y.Z`; release candidates `vX.Y.Z-rc.N`.
  - **MAJOR:** breaking changes for devices or integrators, for example an incompatible device API, a disk layout change that needs a reinstall, a `castmill.conf` format change, or dropping supported hardware.
  - **MINOR:** new features, new hardware support, Electron/Chromium feature upgrades.
  - **PATCH:** bug fixes and security fixes only.
  - While the version is `0.y.z`, breaking changes bump MINOR. `1.0.0` is the v1.0 release from the roadmap.
- The next version follows from the [Conventional Commits](https://www.conventionalcommits.org/) since the last tag: `feat` → MINOR, `fix` → PATCH, `!` or a `BREAKING CHANGE:` footer → MAJOR (see [CONTRIBUTING.md](../../CONTRIBUTING.md#commit-messages)).
- Untagged builds get a pre-release version from `scripts/version.sh`, for example `0.1.1-dev.5.g1a2b3c4` (5 commits after `v0.1.0`). Pre-release identifiers instead of `+build` metadata keep the version valid in `os-release`.
- The image manifest records the Electron, player, kernel and Mesa versions. Components in this repository (`castmill-agent`, the kiosk shell) carry their own SemVer versions.
- Branches: `main` (development) and `release/X.Y` (maintenance for security and patch releases of `X.Y`).

## CI/CD on GitHub Actions

### Runner strategy

Everything runs on **GitHub-hosted runners** (decided, see ADR-0006). Standard runners (4 CPUs, 16 GB RAM, 14 GB SSD) are too small for Yocto, so builds use **larger runners**, which our GitHub Enterprise Cloud organization can provision (Ubuntu x64, 16–64 cores, 600–2040 GB SSD). Every GitHub-hosted job, larger runners included, is limited to 6 hours.

| Runner | Used for | Trust |
|---|---|---|
| **Standard GitHub-hosted** (`ubuntu-latest`) | Lint, docs, unit tests, workflow security checks | Any PR, including forks |
| **Larger runners** (group `castmill-os-builders`, label `castmill-os-builder`; 32-core to start) | Yocto builds, QEMU boot tests | Trusted code only (see below) |
| **Larger runner with a static IP** | Driving the hardware lab (see below) | Trusted artifacts only |
| **Signing** (GitHub-hosted job in a protected environment) | Signing release artifacts through an external HSM/KMS | Release tags only, manual approval |

Larger runners are billed per minute, also for public repositories. A **custom runner image** (Enterprise feature) can pre-install build dependencies later if the container start-up becomes a noticeable cost.

**Hardware lab:** physical devices can't run inside GitHub, but the CI jobs can still. The lab exposes only a test controller (for example a labgrid coordinator controlling power, serial consoles, HDMI capture and network switches). A larger runner with a **static IP address** (Enterprise feature) connects to it over mutual TLS, and the lab firewall only accepts those IPs. Nothing executes jobs inside the lab network, and no self-hosted runners are needed.

### Public-repository safety rules

The repository is public and open to outside contributions, so untrusted code must never reach paid build runners unreviewed, the hardware lab, signing keys or the shared cache:

1. Fork PRs run **only** on standard GitHub-hosted runners, with no secrets. The repo setting "Require approval for workflow runs from fork pull requests" is set to all outside contributors.
2. Yocto builds and lab tests run only on `push` to `main` / `release/*`, on tags, on schedules, and on PRs from branches inside the repository. A fork PR gets a full build only when a maintainer triggers it manually after code review (`workflow_dispatch` with the PR's commit). This also keeps paid larger-runner minutes under maintainer control.
3. **Only trusted builds (pushes to main and release branches) write to the build cache**; other builds restore it read-only, to prevent cache poisoning.
4. The lab controller accepts connections only from the static IPs of our larger runners, with mutual TLS; lab credentials are environment secrets available only to trusted workflows.
5. All third-party actions are **pinned to a commit SHA**. Workflows get minimal `permissions:`. Workflow files are linted (`actionlint`) and security-scanned (`zizmor`).
6. Cloud access uses **OIDC** short-lived credentials; no static cloud keys stored in GitHub secrets.
7. `CODEOWNERS` requires maintainer review for `.github/`, `kas/`, `layers/`, the keyrings and the signing configuration. Branch rulesets on `main` and `release/*` require the CI checks to pass and signed commits.

### Workflows

| Workflow | Trigger | Runner | What it does |
|---|---|---|---|
| `checks.yml` ✅ | every PR, push | standard | Markdown link check, `actionlint`, `zizmor`, yamllint, shellcheck, kas config resolution (later: agent unit tests); Conventional Commits + DCO sign-off on PR commits (`scripts/check-commits.sh`); to add: `reuse lint` |
| `build.yml` ✅ | push to `main`/`release/*`, internal PRs, manual, called by `release.yml` | larger runners (standard for now) | kas build per target (`x86-64`, `rpi5`, `vm-arm64` dev image); images, RAUC bundles + SBOM uploaded as workflow artifacts. Runs on standard runners (x86-64 and arm64) until the larger runners exist (repository variable `CASTMILL_LARGER_RUNNERS=true`); a cold build may need two runs, since the shared-state cache is saved even when a run hits the time limit. |
| `qemu-test.yml` | after `build.yml` | larger runners (KVM) | Boot the x86-64 image in QEMU: health check, A/B update, forced rollback, power-cut simulation |
| `nightly.yml` | schedule (daily) | larger runners (static IP for the lab) | Full build from `main` → `nightly` channel → hardware lab suites |
| `release.yml` ✅ (initial) | tag `vX.Y.Z[-rc.N]` | larger runners → standard | Builds all targets and publishes a **development-signed GitHub pre-release** (images, bundles, SBOMs, `SHA256SUMS`). The full pipeline below (test gates, production signing, attestations, manifest registration) replaces it before v1.0. |
| `cve-watch.yml` | schedule (daily) | GitHub-hosted | Compares the latest release's SBOM against new CVEs; opens an issue if critical |
| `bump-deps.yml` | schedule (weekly) | GitHub-hosted | Opens PRs for new Yocto layer revisions and Electron releases (Dependabot handles action versions) |

### Release pipeline (`release.yml`)

```
tag vX.Y.Z(-rc.N)
   │
   ├─ 1. Build (or promote the exact artifacts already tested in nightly/RC, by content hash)
   ├─ 2. QEMU tests + hardware lab suites (required for release)
   ├─ 3. Sign ──► protected environment "production-signing"
   │             · required reviewers (2 maintainers)
   │             · keys in HSM/cloud KMS, accessed via OIDC; RAUC and Secure Boot signing over PKCS#11
   ├─ 4. Attest  ──► build provenance (actions/attest-build-provenance) for every artifact
   ├─ 5. Publish
   │     · GitHub Release: images (.wic.zst + .bmap), x86 installer, SBOM, CVE report,
   │       license manifest, SHA256SUMS (signed), release notes
   │     · GPL/LGPL source archives (object storage, linked from the release)
   │     · RAUC bundles → object storage / CDN
   │     · Register the release manifest in the Castmill backend (channel: canary for -rc, beta for final)
   │     · Update the Raspberry Pi Imager repository JSON (final releases only)
   └─ 6. Promotion to stable happens in Castmill's rollout engine, not in CI
```

- Each file in a GitHub Release must be **under 2 GiB**. Compressed images are well below that; CI checks artifact sizes before uploading.
- RAUC bundles are served from object storage/CDN, not GitHub Releases, because devices need fast range requests and Castmill controls the rollout.

## Release channels and cadence

| Channel | Audience | Source | Cadence |
|---|---|---|---|
| `nightly` | CI and hardware lab only | `main` | Daily |
| `canary` | Internal devices | `-rc` tags | Weekly |
| `beta` | Opt-in customers | Final tags | Every 2–4 weeks |
| `stable` | Everyone | Promoted from `beta` in Castmill | Monthly feature releases + security releases as needed |

Security releases (`PATCH` bumps from `release/*`) can go straight to `stable` with a fast staged rollout, after the automated QEMU and lab tests pass.

## Signing

- CI never holds production keys. Signing runs in the protected `production-signing` environment and uses an HSM/KMS through OIDC.
- Development keys (also kept outside the repo) sign `nightly` and dev images only. Production devices reject them.
- Key rotation: devices trust the current key and the next one (see [03](03-updates.md)).

## Testing

### Developer testing

Before anything ships, developers run images locally: the `vm-arm64` development VM on Apple Silicon Macs (QEMU with HVF acceleration) for functional tests, and real devices on the desk for media and performance. See [docs/development.md](../development.md).

### Per pull request

- `checks.yml` (all PRs) and `build.yml` + `qemu-test.yml` (trusted PRs, or maintainer-triggered for forks).

### Hardware-in-the-loop lab (nightly and every release candidate)

Devices: every tier-1 model, connected to controllable power (smart PDU/relays), serial console, HDMI capture (to check that frames are rendered), and a network switch that can cut links. **labgrid** is the candidate for controlling power, serial and boot. CI jobs on GitHub-hosted larger runners (static IP) reserve devices through the lab controller, flash the image under test and run the suites remotely.

Test suites:

- **Boot & health:** cold boot to first frame time (NFR-003).
- **Media:** fixed playlist of reference videos per content profile; measure decoder name, CPU, dropped frames, A/V sync.
- **Update:** install from N-1 and N-2; power cut at random points during install; forced health failure → rollback.
- **Power-cut torture:** 100+ random power cuts across boot, playback and update.
- **Network chaos:** link down, DNS failure, NTP blocked, captive portal, proxy.
- **Display:** hotplug, rotation changes, mode changes, CEC power cycle.

Results are uploaded as workflow artifacts and summarized in the job summary. A failing lab suite blocks the release.

### Soak

- Every release candidate runs **30 days** on a lab group, plus the `canary` field fleet, before being promoted to `stable` (NFR-005). Security releases use a shortened soak.

## Open questions

- Whether to build Electron from source or package the official prebuilt binaries (has a big impact on build time).
- Where the hardware lab lives, and who maintains it.
