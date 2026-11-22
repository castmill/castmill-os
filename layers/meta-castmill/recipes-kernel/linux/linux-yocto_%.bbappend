FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
SRC_URI:append = " file://castmill.cfg"
SRC_URI:append:x86-64 = " file://castmill-x86.cfg"
