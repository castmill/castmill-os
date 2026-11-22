# Castmill OS

**An open source, immutable Linux operating system built for digital signage and the [Castmill](https://castmill.io) player.**

> Status: **initial implementation (v0.1)**. Nothing here is production-ready yet. The specifications in [`docs/specs`](docs/specs) describe what we intend to build and are open for discussion; [implementation status](docs/implementation-status.md) tracks what exists today.

## Why

For years we have run Castmill players on general-purpose operating systems such as Android, stock Linux distributions (Arch, Debian, Ubuntu), Windows and iOS. Those systems weren't designed for unattended screens running 24/7, and the same problems keep coming back:

- updates that break the player, or stop halfway and leave the device unbootable
- filesystem corruption after power cuts
- configuration drift between devices that should be identical
- screens that need a person on site to recover
- large attack surfaces and slow security patching
- video acceleration that works on one OS release and silently stops on the next

Castmill OS is an **appliance OS**: one signed image, built for a single job, which updates itself safely and repairs itself.

## Design principles

1. **Immutable and atomic.** The root filesystem is read-only and verified. Updates replace the whole system image in an inactive A/B slot and switch to it atomically. A device is always running a known, tested image.
2. **Never needs a human on site.** Health-checked boots, automatic rollback, watchdogs at every layer, and storage that survives power loss.
3. **Secure by default.** Verified boot chain, signed updates, no inbound ports, per-device identity, automatic security updates.
4. **Fast media.** Hardware-accelerated graphics and video decoding, 4K content, multiple outputs.
5. **Installs at scale.** Flash or install in minutes, then zero-touch enrollment into Castmill.
6. **Small.** Only what a signage player needs. No desktop, no package manager on the device.

## Supported hardware (initial targets)

| Tier | Hardware | Notes |
|---|---|---|
| 1 | Intel N100/N150-class mini PCs (x86-64, UEFI) | VA-API video, 2–3 outputs, TPM 2.0 |
| 1 | Raspberry Pi 5 / Compute Module 5 | 2× 4K outputs, HEVC hardware decoding |
| 2 | Raspberry Pi 4, AMD mini PCs, Rockchip RK3588 | Later |
| dev | arm64 virtual machine (QEMU/UTM on Apple Silicon Macs) | Functional testing during development |

See [Hardware support](docs/specs/09-hardware-support.md).

## Architecture at a glance

```
┌─────────────────────────────────────────────────────────────┐
│ Castmill Player (Electron)          castmill-agent          │
├─────────────────────────────────────────────────────────────┤
│ Kiosk compositor (Wayland) · Mesa / KMS                     │
├─────────────────────────────────────────────────────────────┤
│ systemd · NetworkManager · RAUC · nftables · journald       │
├─────────────────────────────────────────────────────────────┤
│ Linux kernel (read-only rootfs, dm-verity)                  │
├─────────────────────────────────────────────────────────────┤
│ Verified boot (UEFI Secure Boot / Pi signed boot) + A/B     │
└─────────────────────────────────────────────────────────────┘
 Disk:  [ boot ] [ rootfs A ] [ rootfs B ] [ data (encrypted) ]
```

- **Build system:** Yocto Project 6.0 "Wrynose" LTS, configured with kas
- **Updates:** RAUC A/B bundles with signing, HTTP streaming and adaptive (delta-like) downloads
- **Fleet rollout:** managed from the Castmill backend
- **Player runtime:** Electron (current Castmill Linux player)
- **CI/CD:** GitHub Actions (GitHub-hosted larger runners) builds, tests (QEMU + real-hardware lab), signs and publishes every release

## Quick start (development)

Images are built by GitHub Actions for every push. To try the latest one on an Apple Silicon Mac:

```sh
brew install qemu zstd gh
scripts/run-vm.sh fetch   # download the latest vm-arm64 dev image from CI
scripts/run-vm.sh run     # boot it in QEMU with hardware virtualization
```

Building locally (Linux):

```sh
pipx install kas
scripts/gen-dev-keys.sh                      # development signing keys (once)
kas-container build kas/vm-arm64.yml:kas/dev.yml
```

Working on the agent or the player doesn't need Yocto:

```sh
cd agent && go test ./...                    # castmill-agent, incl. an end-to-end test
go run ./cmd/castmill-mock-backend --auto-approve   # development backend (device API)
cd ../player && npm run check                # Electron kiosk shell
```

See the [development guide](docs/development.md) for real hardware, local builds and limitations.

## Documentation

| Spec | Topic |
|---|---|
| [01](docs/specs/01-vision-and-requirements.md) | Vision, scope and requirements |
| [02](docs/specs/02-system-architecture.md) | System architecture |
| [03](docs/specs/03-updates.md) | Updates and rollouts |
| [04](docs/specs/04-security.md) | Security |
| [05](docs/specs/05-reliability.md) | Reliability and self-healing |
| [06](docs/specs/06-display-and-media.md) | Display, graphics and media |
| [07](docs/specs/07-device-management.md) | Device management and remote access |
| [08](docs/specs/08-installation-and-provisioning.md) | Installation and provisioning |
| [09](docs/specs/09-hardware-support.md) | Hardware support |
| [10](docs/specs/10-build-release-testing.md) | Build, release and testing |
| [11](docs/specs/11-roadmap.md) | Roadmap |
| [Device API](docs/device-api.md) | Agent ↔ Castmill backend protocol (draft v1) |
| [Status](docs/implementation-status.md) | What is implemented, per requirement |

Architecture decisions are recorded in [`docs/adr`](docs/adr). Day-to-day development is described in the [development guide](docs/development.md).

## Repository layout

```
castmill-os/
├── README.md
├── .github/
│   ├── workflows/      # checks.yml, build.yml (later: qemu-test, nightly, release, cve-watch)
│   ├── CODEOWNERS
│   └── dependabot.yml
├── docs/
│   ├── specs/          # specifications
│   ├── adr/            # architecture decision records
│   ├── device-api.md   # agent <-> backend protocol
│   ├── implementation-status.md
│   └── development.md  # development guide
├── kas/                # build configurations: x86-64, rpi5, vm-arm64, dev overlay
├── layers/
│   └── meta-castmill/  # distro, image, RAUC bundle, A/B boot, system integration, recipes
├── agent/              # castmill-agent (Go) + development mock backend
├── player/             # Electron kiosk shell hosting the Castmill player
├── scripts/
│   ├── run-vm.sh       # run the development VM on a Mac
│   ├── gen-dev-keys.sh # development signing keys
│   └── check-go-mods.sh
├── installer/          # (planned) USB / network installer
└── tests/              # (planned) QEMU and hardware-lab tests
```

## Contributing

The project is at the specification and prototype stage. Issues and pull requests on GitHub are very welcome; see [CONTRIBUTING.md](CONTRIBUTING.md). Larger decisions go in an ADR (see [`docs/adr/README.md`](docs/adr/README.md)).

Every pull request runs automated checks on GitHub Actions. Full image builds for pull requests from forks are started by a maintainer after review (see [Build, release and testing](docs/specs/10-build-release-testing.md#public-repository-safety-rules)).

## License

Castmill OS uses permissive licenses so integrators and OEMs can adopt it easily ([ADR-0007](docs/adr/0007-licensing.md)):

- build metadata, layers, recipes, scripts and CI: [MIT](LICENSE)
- Castmill software running on the device (`agent/`, `player/`): [Apache-2.0](LICENSES/Apache-2.0.txt)
- documentation and specifications: [CC BY 4.0](LICENSES/CC-BY-4.0.txt)

Per-file licensing is declared in [`REUSE.toml`](REUSE.toml). Images also contain third-party packages under their own licenses; every release publishes the license manifest, SBOM and corresponding source. "Castmill" and the Castmill logo are trademarks and aren't covered by these licenses.

Contributions are welcome under the [DCO](CONTRIBUTING.md#developer-certificate-of-origin-dco); see [CONTRIBUTING.md](CONTRIBUTING.md).
