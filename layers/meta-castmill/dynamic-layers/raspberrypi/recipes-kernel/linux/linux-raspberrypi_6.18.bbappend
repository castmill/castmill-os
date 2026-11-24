# A newer stable release of the Raspberry Pi kernel than meta-raspberrypi pins
# (6.18.33): the kernel's CNA files a CVE for nearly every stable fix, and the
# older pin left ~5800 of them open in the release's CVE report (SEC-009).
# Drop this when meta-raspberrypi catches up.
LINUX_VERSION = "6.18.65"
SRCREV_machine = "3eddc9976f7a4c687e1bf578118d0d91c12c69da"
