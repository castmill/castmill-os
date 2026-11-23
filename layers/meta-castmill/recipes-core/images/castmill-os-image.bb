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
# RAUC verity bundles need the slot image to be a whole number of 4 KiB
# blocks; the default alignment (1 KiB) only gets there by chance.
IMAGE_ROOTFS_ALIGNMENT = "4"

# A/B disk layout and bootloader for UEFI targets (x86-64, vm-arm64).
WKS_FILE:genericx86-64 = "castmill-efi-ab.wks.in"
WKS_FILE:genericarm64 = "castmill-efi-ab.wks.in"
# Raspberry Pi 5: one boot partition per slot, firmware tryboot (ADR-0009).
WKS_FILE:raspberrypi5 = "castmill-rpi.wks.in"

# A/B updates through RAUC (CASTMILL_AB = "0" for a platform without A/B boot).
CASTMILL_AB ??= "1"
# Bootloader side of A/B per platform: GRUB with boot counting on UEFI
# targets, the firmware's tryboot on the Raspberry Pi.
CASTMILL_AB_PACKAGES ?= "rauc grub-editenv"
CASTMILL_AB_PACKAGES:raspberrypi5 = "rauc castmill-tryboot"
CASTMILL_BOOTCONF ?= "castmill-bootconf"
CASTMILL_BOOTCONF:raspberrypi5 = "castmill-tryboot"
do_image_wic[depends] += "${@d.getVar('CASTMILL_BOOTCONF') + ':do_deploy' if d.getVar('CASTMILL_AB') == '1' else ''}"

IMAGE_INSTALL = " \
    packagegroup-core-boot \
    kernel-image kernel-modules \
    ${CASTMILL_FIRMWARE} \
    castmill-base \
    castmill-agent \
    castmill-session \
    castmill-player \
    ${@d.getVar('CASTMILL_AB_PACKAGES') if d.getVar('CASTMILL_AB') == '1' else ''} \
    networkmanager networkmanager-nmcli wpa-supplicant \
    ca-certificates tzdata \
    weston mesa-megadriver ${CASTMILL_VIDEO_DRIVERS} \
    liberation-fonts \
    ${@'castmill-dev' if d.getVar('CASTMILL_DEV_IMAGE') == '1' else ''} \
    ${CORE_IMAGE_EXTRA_INSTALL} \
"

# Firmware and VA-API video drivers per hardware family (set in the kas
# target files).
CASTMILL_FIRMWARE ??= ""
CASTMILL_VIDEO_DRIVERS ??= ""

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

# Raspberry Pi: the boot files (IMAGE_BOOT_FILES, as wic copies them to
# boot_a) also go into the root filesystem, so an update can write the new
# slot's boot partition from it (castmill-bootfs-install, ADR-0009).
python castmill_rpi_bootfiles() {
    import glob, os, re, shutil
    deploy = d.getVar('DEPLOY_DIR_IMAGE')
    dest = os.path.join(d.getVar('IMAGE_ROOTFS'), 'usr/lib/castmill/boot')
    os.makedirs(dest, exist_ok=True)

    def copy(src, name):
        target = os.path.join(dest, name)
        os.makedirs(os.path.dirname(target), exist_ok=True)
        if os.path.isdir(src):
            shutil.copytree(src, target, dirs_exist_ok=True)
        else:
            shutil.copyfile(src, target)

    for entry in re.findall(r'[\w;\-\./\*]+', d.getVar('IMAGE_BOOT_FILES')):
        src, _, dst = entry.partition(';')
        if '*' in src:
            for f in glob.glob(os.path.join(deploy, src)):
                base = os.path.basename(f)
                copy(f, os.path.join(dst, base) if dst else base)
        else:
            copy(os.path.join(deploy, src), dst or src)
    if not os.path.exists(os.path.join(dest, 'cmdline.txt')):
        bb.fatal('castmill_rpi_bootfiles: no cmdline.txt in IMAGE_BOOT_FILES')
}
ROOTFS_POSTPROCESS_COMMAND:append:raspberrypi5 = " castmill_rpi_bootfiles"
do_rootfs[depends] += "${@'virtual/kernel:do_deploy rpi-bootfiles:do_deploy' if 'raspberrypi' in d.getVar('MACHINE') else ''}"

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
