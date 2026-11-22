SUMMARY = "Castmill OS update bundle"
DESCRIPTION = "Signed RAUC bundle with the castmill-os-image root filesystem \
(spec 03, ADR-0002). Streamed by devices over HTTPS."
LICENSE = "MIT"

inherit bundle

RAUC_BUNDLE_COMPATIBLE = "${CASTMILL_COMPATIBLE}"
RAUC_BUNDLE_VERSION = "${DISTRO_VERSION}"
RAUC_BUNDLE_DESCRIPTION = "Castmill OS ${DISTRO_VERSION} for ${MACHINE}"
# verity: required for HTTP streaming and adaptive updates (UPD-006).
RAUC_BUNDLE_FORMAT = "verity"

RAUC_BUNDLE_SLOTS = "rootfs"
RAUC_SLOT_rootfs = "castmill-os-image"
RAUC_SLOT_rootfs[fstype] = "ext4"
RAUC_SLOT_rootfs[adaptive] = "block-hash-index"

# Development signing key from CASTMILL_KEYS_DIR (never in the repository).
# Release bundles are re-signed with the production key in the protected
# signing environment (spec 10, release.yml).
RAUC_KEY_FILE = "${CASTMILL_KEYS_DIR}/rauc-signing.key.pem"
RAUC_CERT_FILE = "${CASTMILL_KEYS_DIR}/rauc-signing.cert.pem"
