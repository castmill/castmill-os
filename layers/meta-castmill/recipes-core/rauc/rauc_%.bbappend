# D-Bus service (castmill-agent), HTTP streaming with mTLS, JSON status output.
PACKAGECONFIG:append = " service streaming network json"
# The bootloader tools come with the image (CASTMILL_AB_PACKAGES): grub-editenv
# for GRUB, castmill-tryboot on the Raspberry Pi.
