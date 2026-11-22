SUMMARY = "Castmill OS A/B bootloader configuration (GRUB)"
DESCRIPTION = "grub.cfg with A/B slot selection and boot counting, and the \
initial grubenv, deployed for the castmill-efi-ab wks layout."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = "file://castmill-grub.cfg.in"
S = "${UNPACKDIR}"

DEPENDS = "grub-native"
INHIBIT_DEFAULT_DEPS = "1"
inherit deploy nopackages

do_configure[noexec] = "1"

do_compile() {
    sed -e 's|@CMDLINE@|${CASTMILL_KERNEL_CMDLINE}|' ${UNPACKDIR}/castmill-grub.cfg.in > ${B}/castmill-grub.cfg
    # Factory state: slot A is good, slot B is empty.
    rm -f ${B}/castmill-grubenv
    grub-editenv ${B}/castmill-grubenv create
    grub-editenv ${B}/castmill-grubenv set ORDER="A B" A_OK=1 A_TRY=0 B_OK=0 B_TRY=0
}
do_compile[vardeps] += "CASTMILL_KERNEL_CMDLINE"

do_deploy() {
    install -m 0644 ${B}/castmill-grub.cfg ${DEPLOYDIR}/castmill-grub.cfg
    install -m 0644 ${B}/castmill-grubenv ${DEPLOYDIR}/castmill-grubenv
}
addtask deploy after do_compile before do_build

COMPATIBLE_HOST = "(x86_64|aarch64).*-linux"
