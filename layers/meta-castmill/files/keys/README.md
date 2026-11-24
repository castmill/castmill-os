# Production keyring

`production-keyring.pem` is the **public** certificate of the Castmill OS
production root CA. Release images (`release.yml`) trust only this
certificate, so they only install update bundles signed with the production
key. Development images trust the development keys instead.

It is created once with `scripts/gen-prod-keys.sh new` (the root CA key itself
never enters this repository). See [docs/release-signing.md](../../../../docs/release-signing.md).
