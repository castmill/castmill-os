# Castmill OS RAUC configuration: A/B rootfs slots, GRUB backend, keyring.
FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
SRC_URI:append = " file://system-rpi.conf"

# GRUB on UEFI targets; the firmware's tryboot on the Raspberry Pi (ADR-0009).
CASTMILL_RAUC_SYSTEM_CONF ?= "system.conf"
CASTMILL_RAUC_SYSTEM_CONF:raspberrypi5 = "system-rpi.conf"
# compatible= and the bootloader differ per machine.
PACKAGE_ARCH = "${MACHINE_ARCH}"

# The keyring comes from outside the repository (CASTMILL_KEYS_DIR): the
# development keyring for dev/nightly images, the production keyring for
# release images. Production devices therefore reject development bundles.
CASTMILL_RAUC_KEYRING ?= "${CASTMILL_KEYS_DIR}/rauc-keyring.pem"
RAUC_KEYRING_URI = ""

do_install() {
    if [ ! -f "${CASTMILL_RAUC_KEYRING}" ]; then
        bbfatal "RAUC keyring not found: ${CASTMILL_RAUC_KEYRING}. Run scripts/gen-dev-keys.sh or set CASTMILL_KEYS_DIR."
    fi
    install -d ${D}${nonarch_libdir}/rauc
    sed -e 's|@COMPATIBLE@|${CASTMILL_COMPATIBLE}|' \
        -e 's|@UUID_A@|${CASTMILL_UUID_ROOTFS_A}|' \
        -e 's|@UUID_B@|${CASTMILL_UUID_ROOTFS_B}|' \
        ${UNPACKDIR}/${CASTMILL_RAUC_SYSTEM_CONF} > ${D}${nonarch_libdir}/rauc/system.conf
    install -m 0644 ${CASTMILL_RAUC_KEYRING} ${D}${nonarch_libdir}/rauc/keyring.pem
}
do_install[file-checksums] += "${CASTMILL_RAUC_KEYRING}:False"
do_install[vardeps] += "CASTMILL_COMPATIBLE CASTMILL_UUID_ROOTFS_A CASTMILL_UUID_ROOTFS_B CASTMILL_RAUC_SYSTEM_CONF"
