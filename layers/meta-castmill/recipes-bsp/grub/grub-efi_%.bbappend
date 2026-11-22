# Modules used by castmill-grub.cfg (A/B selection): regexp to find the boot
# disk, probe to read the slot's partition UUID, loadenv for grubenv.
GRUB_BUILDIN:append = " regexp probe loadenv echo test"
