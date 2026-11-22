SUMMARY = "Castmill OS development image additions"
DESCRIPTION = "Firewall exceptions for SSH and ping. Installed only when \
CASTMILL_DEV_IMAGE = 1 (kas/dev.yml); never part of production images."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = "file://ssh.nft"
S = "${UNPACKDIR}"

inherit allarch

do_install() {
    install -D -m 0644 ${S}/ssh.nft ${D}${sysconfdir}/nftables/castmill-input.d/50-dev-ssh.nft
}

python () {
    if d.getVar('CASTMILL_DEV_IMAGE') != '1':
        raise bb.parse.SkipRecipe("development images only (CASTMILL_DEV_IMAGE != 1)")
}
