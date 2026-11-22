# ADR-0007: Licensing

- Status: Accepted
- Date: 2026-11-22

## Context

Castmill (the CMS and backend) is licensed under AGPL-3.0. Castmill OS has a different job: the **top priority is making life easy for integrators and OEMs**, so that Castmill OS becomes the obvious way to run Castmill on Pi and Intel hardware. Integrators need licenses their legal teams approve quickly, clear machine-readable licensing, and images without obligations that conflict with locked-down devices.

## Decision

1. **Permissive licenses for everything in this repository:**

   | Part | License | Why |
   |---|---|---|
   | Build metadata (kas, layers, recipes, distro config), scripts, CI | **MIT** | Yocto convention (OE-core metadata is MIT); easy to upstream and to build vendor layers on top |
   | Castmill software running on the device: `castmill-agent` (incl. health check, `agent/`), the Electron kiosk shell (`player/`), later `castmill-videod` | **Apache-2.0** | Permissive, with an explicit patent grant that OEMs and larger customers expect |
   | Documentation and specifications | **CC BY 4.0** | Reusable with attribution |

2. **Machine-readable licensing** with [REUSE](https://reuse.software) (`REUSE.toml`, `LICENSES/`), so integrators can verify licensing automatically.
3. **Contributions under the DCO** (`Signed-off-by`), no CLA.
4. **Brand protection through a trademark policy:** anyone can use and ship the code; only official builds may be called "Castmill OS" or use the Castmill logo (policy to be written).
5. **Production images exclude GPL-3.0, LGPL-3.0 and AGPL-3.0 packages** (Yocto `INCOMPATIBLE_LICENSE`), so OEMs can ship locked-down secure-boot devices without GPLv3 installation-information obligations. Exceptions need an ADR. Development images may include GPLv3 tools.
6. **License compliance is part of every release:** license manifest, SPDX SBOM, and the corresponding source for all GPL/LGPL packages (Yocto archiver), published next to the images.

### What the image does *not* contain

The Castmill player itself (the AGPL web app from the Castmill repository) is **not** part of the image: the kiosk shell loads it from the Castmill backend at runtime. This keeps AGPL code out of production images (point 5). If the player is ever bundled into the image (for example for offline-first start-up), that needs a new decision, because AGPL-3.0 belongs to the GPLv3 family.

## Alternatives considered

- **AGPL-3.0 for the agent (matching Castmill):** prevents competing CMSs from reusing our device software without contributing back, but slows OEM adoption and would put a GPLv3-family component into images that should be GPLv3-free.
- **GPL-2.0 for Castmill code:** familiar from Linux, but adds review friction for integrators without protecting anything important (the backend stays AGPL).
- **CLA with dual licensing:** keeps the option to relicense later, but deters contributors; not needed with permissive licenses.

## Consequences

- Competitors can use Castmill OS with their own backends. That's accepted: the protected value is in the Castmill backend (AGPL) and the brand.
- Excluding GPLv3 limits some packages (for example GNU bash and coreutils versions under GPLv3). Production images use alternatives such as BusyBox or toybox; the impact is checked in Phase 0.
- The release pipeline (spec 10) must publish compliance artifacts, and CI should run `reuse lint`.
- The copyright holder is "The Castmill OS contributors"; this can be changed to the company's legal entity name before the first release.
