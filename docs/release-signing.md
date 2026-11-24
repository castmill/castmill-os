# Release signing

Every Castmill OS update bundle is signed, and a device only installs bundles whose signature it trusts. Releases are signed with the **production** key; everything else (pushes, pull requests, development images) with the **development** key. Production images don't accept development bundles, and development images don't accept production ones.

## How it fits together

| Key | What it is | Where it lives |
|---|---|---|
| **Root CA** | Approves signing keys. Its public certificate is in every release image, so devices trust whatever it approved. | The encrypted key file and its passphrase in the password managers of two maintainers. Never on a server or in CI. |
| **Root certificate** | The public half of the root CA. | `layers/meta-castmill/files/keys/production-keyring.pem` in this repository |
| **Signing key** | Signs the bundles of each release. Valid for 2 years. | GitHub secrets `CASTMILL_PROD_RAUC_KEY` and `CASTMILL_PROD_RAUC_CERT` in the protected **production-signing** environment |

A release (`release.yml`, started by a `vX.Y.Z` tag):

1. **build** builds all targets. Images trust only the root certificate; bundles are signed with the development key for now.
2. **sign** waits for a maintainer's approval, then re-signs every bundle with the production key (`rauc resign`) and checks it against the root certificate. It's the only job that can read the production signing key, and it runs no build code.
3. **publish** creates the GitHub release with the production-signed bundles.

## One-time setup

You need `openssl` and the GitHub CLI (`gh`) on your own computer.

1. **Create the GitHub environment.** In the repository's *Settings → Environments*, create `production-signing`. Add yourself (and your partner) as **required reviewers**, and limit it to tags matching `v*`.
2. **Create the keys:**

   ```sh
   scripts/gen-prod-keys.sh new ~/castmill-keys
   ```

   It prints a passphrase once. Copy it right away.
3. **Store the root CA** in your password manager: the file `root-ca.key.enc.pem` and the passphrase. Give your partner the same two things for their password manager. Two copies in two places mean losing one is no problem, and the file is useless without the passphrase.
4. **Commit the root certificate** (it's public):

   ```sh
   cp ~/castmill-keys/root-ca.pem layers/meta-castmill/files/keys/production-keyring.pem
   git add layers/meta-castmill/files/keys/production-keyring.pem
   git commit -s -m "feat: production keyring"
   ```

5. **Upload the signing key** to the environment:

   ```sh
   gh secret set CASTMILL_PROD_RAUC_KEY  --env production-signing < ~/castmill-keys/signing.key.pem
   gh secret set CASTMILL_PROD_RAUC_CERT --env production-signing < ~/castmill-keys/signing.cert.pem
   ```

6. **Delete** `~/castmill-keys` from your computer.

From the next release on, approve the **sign** job when GitHub asks (the release's workflow run shows *Review deployments*).

::: warning Devices running development-signed images
Devices flashed with images from before this setup (or with development images) trust the development key and won't install production-signed updates. Flash them once with a production-signed release.
:::

## Renewing the signing key

The signing certificate is valid for 2 years. Renew it before it expires (a release made with an expired certificate is rejected by devices whose clock is right):

1. Restore `root-ca.key.enc.pem` and `root-ca.pem` from the password manager into an empty directory, for example `~/castmill-keys`.
2. Run `scripts/gen-prod-keys.sh renew ~/castmill-keys` and type the passphrase.
3. Upload the new `signing.key.pem` and `signing.cert.pem` with the two `gh secret set` commands above.
4. Delete the directory.

Devices need no update: they trust the root CA, not a particular signing key.

## If something goes wrong

| What happened | What to do |
|---|---|
| A maintainer's password manager is lost | Nothing urgent: the other copy works. Make a new second copy. |
| Both copies of the root CA are lost | No new signing keys can be issued once the current one expires. Create a new root CA, ship it in a release signed with the current signing key (devices then trust both), and move to the new one. |
| The signing key leaked (for example a compromised GitHub account) | Delete the GitHub secrets, issue a new signing key with `renew`, and plan to remove trust in the old key: a release whose keyring adds a certificate revocation list for the leaked certificate. |
| The root CA key leaked | Treat every device as untrusted for updates: create a new root CA and re-flash devices. Keep the passphrase and the file separate from anything else to make this unlikely. |
