SUMMARY = "Castmill OS display session"
DESCRIPTION = "Weston kiosk-shell session as the player user, one player \
instance per connected output, display hotplug and crash-loop escalation. \
See docs/specs/06-display-and-media.md and ADR-0004."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://castmill-session \
    file://castmill-outputs \
    file://castmill-session.service \
    file://castmill-player@.service \
    file://castmill-session-restart.service \
    file://castmill-outputs.service \
    file://99-castmill-drm.rules \
    file://castmill-session.pam \
"
S = "${UNPACKDIR}"

inherit systemd allarch

SYSTEMD_SERVICE:${PN} = "castmill-session.service"

do_install() {
    install -D -m 0755 ${S}/castmill-session ${D}${libexecdir}/castmill/castmill-session
    install -D -m 0755 ${S}/castmill-outputs ${D}${libexecdir}/castmill/castmill-outputs
    install -d ${D}${systemd_system_unitdir}
    for u in castmill-session.service castmill-player@.service castmill-session-restart.service castmill-outputs.service; do
        install -m 0644 ${S}/$u ${D}${systemd_system_unitdir}/
    done
    install -D -m 0644 ${S}/99-castmill-drm.rules ${D}${nonarch_base_libdir}/udev/rules.d/99-castmill-drm.rules
    install -D -m 0644 ${S}/castmill-session.pam ${D}${sysconfdir}/pam.d/castmill-session
}

FILES:${PN} += "${libexecdir}/castmill ${systemd_system_unitdir} ${nonarch_base_libdir}/udev/rules.d"

# kbd: chvt.
RDEPENDS:${PN} = "weston castmill-agent castmill-player castmill-base libpam pam-plugin-permit pam-plugin-loginuid kbd"
