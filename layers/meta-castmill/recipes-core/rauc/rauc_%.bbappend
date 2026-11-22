# D-Bus service (castmill-agent), HTTP streaming with mTLS, JSON status output.
PACKAGECONFIG:append = " service streaming network json"

# The GRUB backend edits grubenv with grub-editenv.
RDEPENDS:${PN} += "grub-editenv"
