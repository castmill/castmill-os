SUMMARY = "Castmill OS image"
DESCRIPTION = "Immutable Castmill OS image for digital signage players. \
Installed into the A/B rootfs slots; see docs/specs/02-system-architecture.md."
LICENSE = "MIT"

inherit core-image

# Read-only root filesystem (writable state lives on the data partition).
IMAGE_FEATURES += "read-only-rootfs"
IMAGE_LINGUAS = ""

# ext4 for the RAUC bundle (one slot image); wic for flashing/installing.
IMAGE_FSTYPES:append = " ext4"

# A/B disk layout and bootloader for UEFI targets (x86-64, vm-arm64).
WKS_FILE:genericx86-64 = "castmill-efi-ab.wks.in"
WKS_FILE:genericarm64 = "castmill-efi-ab.wks.in"
# Raspberry Pi 5: same partitions; A/B boot is a Phase 0 decision (spec 03).
WKS_FILE:raspberrypi5 = "castmill-rpi.wks.in"
do_image_wic[depends] += "${@'castmill-bootconf:do_deploy' if d.getVar('CASTMILL_AB') == '1' else ''}"

# A/B updates through RAUC (CASTMILL_AB = "0" until a platform's A/B boot works).
CASTMILL_AB ??= "1"
CASTMILL_AB:raspberrypi5 ??= "0"

IMAGE_INSTALL = " \
    packagegroup-core-boot \
    kernel-image kernel-modules \
    ${CASTMILL_FIRMWARE} \
    castmill-base \
    castmill-agent \
    castmill-session \
    castmill-player \
    ${@'rauc grub-editenv' if d.getVar('CASTMILL_AB') == '1' else ''} \
    networkmanager networkmanager-nmcli wpa-supplicant \
    ca-certificates tzdata \
    weston mesa-megadriver \
    liberation-fonts \
    ${@'castmill-dev' if d.getVar('CASTMILL_DEV_IMAGE') == '1' else ''} \
    ${CORE_IMAGE_EXTRA_INSTALL} \
"

# Firmware per hardware family (set in the kas target files).
CASTMILL_FIRMWARE ??= ""

SYSTEMD_DEFAULT_TARGET = "graphical.target"

# castmill-session starts Weston; weston-init's own weston.service would compete.
BAD_RECOMMENDATIONS += "weston-init"

# Fixed name for the slot's kernel so the A/B bootloader configuration
# doesn't depend on the kernel version.
castmill_kernel_link() {
    k=$(ls ${IMAGE_ROOTFS}/boot/${KERNEL_IMAGETYPE}-* 2>/dev/null | head -n1)
    if [ -z "$k" ]; then
        bbfatal "no kernel found in ${IMAGE_ROOTFS}/boot"
    fi
    ln -sf "$(basename "$k")" ${IMAGE_ROOTFS}/boot/castmill-kernel
}
ROOTFS_POSTPROCESS_COMMAND += "castmill_kernel_link"

# Image manifest: versions of the key components (spec 10 "Versioning").
castmill_manifest() {
    cat > ${IMAGE_ROOTFS}${sysconfdir}/castmill-release <<EOM
CASTMILL_VERSION=${DISTRO_VERSION}
MACHINE=${MACHINE}
COMPATIBLE=${CASTMILL_COMPATIBLE}
DEV_IMAGE=${CASTMILL_DEV_IMAGE}
BUILD=${DATETIME}
EOM
}
castmill_manifest[vardepsexclude] += "DATETIME"
ROOTFS_POSTPROCESS_COMMAND += "castmill_manifest"
