SUMMARY = "Raspberry Pi 5 tryboot A/B backend for RAUC"
DESCRIPTION = "RAUC custom bootloader backend for the Raspberry Pi firmware's \
tryboot (autoboot.txt), and the post-install handler that writes a slot's \
boot partition from its root filesystem (ADR-0009). Also deploys the initial \
autoboot.txt for the castmill-rpi wks layout."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://castmill-tryboot \
    file://castmill-bootfs-install \
    file://tryboot.conf.in \
"
S = "${UNPACKDIR}"

COMPATIBLE_MACHINE = "^rpi$"

inherit deploy
PACKAGE_ARCH = "${MACHINE_ARCH}"

do_configure[noexec] = "1"
do_compile[noexec] = "1"

do_install() {
    install -D -m 0755 ${S}/castmill-tryboot ${D}${nonarch_libdir}/castmill/castmill-tryboot
    install -D -m 0755 ${S}/castmill-bootfs-install ${D}${nonarch_libdir}/castmill/castmill-bootfs-install
    sed -e 's|@UUID_BOOT_A@|${CASTMILL_UUID_BOOT_A}|' \
        -e 's|@UUID_BOOT_B@|${CASTMILL_UUID_BOOT_B}|' \
        -e 's|@UUID_ROOTFS_A@|${CASTMILL_UUID_ROOTFS_A}|' \
        -e 's|@UUID_ROOTFS_B@|${CASTMILL_UUID_ROOTFS_B}|' \
        ${S}/tryboot.conf.in > ${D}${nonarch_libdir}/castmill/tryboot.conf
}
do_install[vardeps] += "CASTMILL_UUID_BOOT_A CASTMILL_UUID_BOOT_B CASTMILL_UUID_ROOTFS_A CASTMILL_UUID_ROOTFS_B"

# Factory state: slot A (partition 2) committed, slot B (partition 3) for trials.
do_deploy() {
    printf '[all]\ntryboot_a_b=1\nboot_partition=2\n[tryboot]\nboot_partition=3\n' \
        > ${DEPLOYDIR}/castmill-autoboot.txt
}
addtask deploy after do_install before do_build

FILES:${PN} = "${nonarch_libdir}/castmill"
# mkfs.vfat for the boot partitions; awk, mount and sed come from BusyBox.
RDEPENDS:${PN} = "dosfstools"
