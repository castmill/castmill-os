SUMMARY = "Castmill OS installer stick for x86-64 PCs"
DESCRIPTION = "Bootable USB stick image that writes Castmill OS to a PC's \
internal disk, copies castmill.conf from the stick and powers off \
(spec 08, x86-64 USB installer). Carries castmill-os-image."
LICENSE = "MIT"

inherit core-image

# A minimal system: BusyBox, zstd and the installer script as init. No
# systemd, no kernel modules (the drivers it needs are built in:
# recipes-kernel/linux/files/castmill-x86.cfg).
IMAGE_INSTALL = "base-files castmill-installer"
IMAGE_FEATURES = ""
IMAGE_LINGUAS = ""
NO_RECOMMENDATIONS = "1"
IMAGE_ROOTFS_EXTRA_SPACE = "0"

COMPATIBLE_MACHINE = "genericx86-64"

# Flashed with Raspberry Pi Imager, balenaEtcher, Rufus or bmaptool; CI
# publishes it as castmill-os-installer-<machine>.img.xz.
IMAGE_FSTYPES = "wic.xz wic.bmap"
WKS_FILE = "castmill-installer.wks.in"

# Files on the CASTMILL partition (wic bootimg_partition), from DEPLOY_DIR_IMAGE.
CASTMILL_PAYLOAD_IMAGE = "castmill-os-image-${MACHINE}.rootfs"
IMAGE_BOOT_FILES = " \
    ${CASTMILL_PAYLOAD_IMAGE}.wic.zst;castmill-os.wic.zst \
    ${CASTMILL_PAYLOAD_IMAGE}.wic.bmap;castmill-os.wic.bmap \
    castmill-installer/README.txt;README.txt \
    castmill-installer/castmill.conf.example;castmill.conf.example \
"
do_image_wic[depends] += " \
    castmill-os-image:do_image_complete \
    castmill-installer:do_deploy \
    castmill-bootconf:do_deploy \
    virtual/kernel:do_deploy \
"
