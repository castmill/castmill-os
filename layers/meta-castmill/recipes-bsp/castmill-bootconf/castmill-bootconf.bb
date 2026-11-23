SUMMARY = "Castmill OS bootloader configuration (GRUB)"
DESCRIPTION = "grub.cfg with A/B slot selection and boot counting, and the \
initial grubenv, deployed for the castmill-efi-ab wks layout; and the \
grub.cfg of the x86-64 installer stick (castmill-installer.wks.in)."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = "file://castmill-grub.cfg.in file://castmill-installer-grub.cfg.in"
S = "${UNPACKDIR}"

DEPENDS = "grub-native"
INHIBIT_DEFAULT_DEPS = "1"
inherit deploy nopackages

do_configure[noexec] = "1"

do_compile() {
    # Boot logo as GRUB echo commands (single-quoted: printed literally).
    if LC_ALL=C grep -q "[^ -~]\|'" "${CASTMILL_LOGO}"; then
        bbfatal "${CASTMILL_LOGO}: the logo must be printable ASCII without single quotes"
    fi
    sed -e "s/^/echo '/" -e "s/\$/'/" "${CASTMILL_LOGO}" > ${B}/logo.grub
    sed -e 's|@CMDLINE@|${CASTMILL_KERNEL_CMDLINE}|' \
        -e "/@LOGO@/{
r ${B}/logo.grub
d
}" ${UNPACKDIR}/castmill-grub.cfg.in > ${B}/castmill-grub.cfg
    sed -e 's|@UUID_INSTALLER@|${CASTMILL_UUID_INSTALLER}|' \
        -e "/@LOGO@/{
r ${B}/logo.grub
d
}" ${UNPACKDIR}/castmill-installer-grub.cfg.in > ${B}/castmill-installer-grub.cfg
    # Factory state: slot A is good, slot B is empty.
    rm -f ${B}/castmill-grubenv
    grub-editenv ${B}/castmill-grubenv create
    grub-editenv ${B}/castmill-grubenv set ORDER="A B" A_OK=1 A_TRY=0 B_OK=0 B_TRY=0
}
do_compile[vardeps] += "CASTMILL_KERNEL_CMDLINE CASTMILL_LOGO CASTMILL_UUID_INSTALLER"
do_compile[file-checksums] += "${CASTMILL_LOGO}:True"

do_deploy() {
    install -m 0644 ${B}/castmill-grub.cfg ${DEPLOYDIR}/castmill-grub.cfg
    install -m 0644 ${B}/castmill-grubenv ${DEPLOYDIR}/castmill-grubenv
    install -m 0644 ${B}/castmill-installer-grub.cfg ${DEPLOYDIR}/castmill-installer-grub.cfg
}
addtask deploy after do_compile before do_build

COMPATIBLE_HOST = "(x86_64|aarch64).*-linux"
