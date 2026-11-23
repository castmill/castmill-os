SUMMARY = "Castmill OS installer for x86-64 PCs"
DESCRIPTION = "Init script of the installer stick (castmill-os-installer \
image): writes Castmill OS to the internal disk, copies castmill.conf and \
powers off. Also deploys the files shown on the stick's CASTMILL drive."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://castmill-install \
    file://README.txt \
    file://castmill.conf.example \
"
S = "${UNPACKDIR}"

inherit allarch deploy

do_configure[noexec] = "1"
do_compile[noexec] = "1"

do_install() {
    install -D -m 0755 ${S}/castmill-install ${D}${sbindir}/castmill-install
    install -D -m 0644 ${CASTMILL_LOGO} ${D}${datadir}/castmill/logo.txt
    echo "${DISTRO_VERSION}" > ${D}${datadir}/castmill/version
    # Mount points (the installer's root filesystem is read-only).
    install -d ${D}/mnt/payload ${D}/mnt/target
}
do_install[vardeps] += "DISTRO_VERSION CASTMILL_LOGO"
do_install[file-checksums] += "${CASTMILL_LOGO}:True"

do_deploy() {
    install -D -m 0644 ${S}/README.txt ${DEPLOYDIR}/castmill-installer/README.txt
    install -D -m 0644 ${S}/castmill.conf.example ${DEPLOYDIR}/castmill-installer/castmill.conf.example
}
addtask deploy after do_install before do_build

FILES:${PN} += "/mnt"
# The installer runs as init with BusyBox: blkid -L, blockdev --rereadpt, zstd.
RDEPENDS:${PN} = "busybox zstd util-linux-blkid util-linux-blockdev"
