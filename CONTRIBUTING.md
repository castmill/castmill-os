# Contributing to Castmill OS

Thanks for your interest! Castmill OS is developed in the open on GitHub. Bug reports, ideas, documentation fixes and code are all welcome.

## Before you start

- **Small fixes** (typos, docs, small bugs): open a pull request directly.
- **Larger changes** (new hardware support, architecture changes): open an issue first. Decisions with long-term impact are recorded as [ADRs](docs/adr/README.md).
- **Security issues:** don't open a public issue; see `SECURITY.md` (coming soon).

## Licensing of contributions

Castmill OS uses permissive licenses so that integrators and OEMs can adopt it easily ([ADR-0007](docs/adr/0007-licensing.md)):

| Part | License |
|---|---|
| Build metadata, layers, recipes, kas files, scripts, CI | MIT |
| Castmill software running on the device (`agent/`, `player/`, later `videod/`) | Apache-2.0 |
| Documentation and specifications (`docs/`, `README.md`, this file) | CC BY 4.0 |

By contributing, you agree that your contribution is licensed under the license that applies to the files you change, as listed in [`REUSE.toml`](REUSE.toml). We don't require a Contributor License Agreement.

## Developer Certificate of Origin (DCO)

Every commit must be signed off, certifying that you wrote the change or otherwise have the right to submit it under the project's licenses, as described in the [Developer Certificate of Origin](https://developercertificate.org/):

```
Signed-off-by: Your Name <you@example.com>
```

Add it automatically with `git commit -s`. Pull requests without a sign-off on every commit can't be merged.

## Commit messages

Every commit follows [Conventional Commits 1.0.0](https://www.conventionalcommits.org/) and carries a DCO sign-off:

```
<type>(<scope>)!: <summary>

<body: what and why>

Signed-off-by: Your Name <you@example.com>
```

- **type:** `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore` or `revert`.
- **scope** (optional): the area changed, for example `agent`, `player`, `meta-castmill`, `kas`, `ci`, `docs`.
- **breaking changes:** add `!` after the type/scope and explain in a `BREAKING CHANGE:` footer. For Castmill OS that includes device API changes, disk layout changes that need a reinstall, `castmill.conf` format changes and dropped hardware support.
- Summary in the imperative ("add", not "added"), lower case, no trailing period, at most 100 characters.

CI checks every commit of a pull request (`scripts/check-commits.sh`). Run it locally with `scripts/check-commits.sh origin/main HEAD`.

## Versioning

Castmill OS releases follow [Semantic Versioning 2.0.0](https://semver.org/) (see [spec 10](docs/specs/10-build-release-testing.md#versioning)). The commit types decide the next version: `feat` → minor, `fix` → patch, a breaking change → major (minor while the version is `0.y.z`). `scripts/version.sh` prints the version of the current commit.

## Third-party code

- Don't copy code from other projects into this repository without checking that its license is compatible (MIT, BSD, Apache-2.0 and similar are fine; ask first for anything else).
- Recipes for upstream software must declare the correct `LICENSE` and `LIC_FILES_CHKSUM`.
- Production images must not include GPL-3.0 or LGPL-3.0 licensed packages unless an ADR approves an exception (see [ADR-0007](docs/adr/0007-licensing.md)).

## Pull requests

- Keep pull requests focused on one change.
- CI checks (`checks.yml`) must pass. Image builds for pull requests from forks are started by a maintainer after review.
- Update the relevant spec or ADR when you change behaviour.

## Development

See the [development guide](docs/development.md) for building images and running them on your machine.
