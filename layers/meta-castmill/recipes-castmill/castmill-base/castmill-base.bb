SUMMARY = "Castmill OS base system integration"
DESCRIPTION = "Users, data partition setup and mounts, persistent state on a \
read-only rootfs, watchdog, journal, zram, firewall, NetworkManager, polkit \
and D-Bus policy. See docs/specs/02, 04 and 05."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://castmill-data-setup \
    file://castmill-data-setup.service \
    file://castmill-data-layout.service \
    file://castmill-data.tmpfiles \
    file://data.mount \
    file://boot.mount \
    file://var-lib-systemd.mount \
    file://var-lib-NetworkManager.mount \
    file://var-volatile-log-journal.mount \
    file://system.conf \
    file://journald.conf \
    file://journal-flush.conf \
    file://rauc.conf \
    file://castmill-zram \
    file://castmill-zram.service \
    file://castmill.nft \
    file://castmill-firewall.service \
    file://NetworkManager.conf \
    file://castmill.rules \
    file://castmill-rauc.dbus.conf \
    file://sysctl.conf \
    file://castmill-banner \
    file://castmill-banner.service \
"
S = "${UNPACKDIR}"

inherit systemd useradd allarch

# Fixed IDs keep file ownership on /data stable across OS versions.
USERADD_PACKAGES = "${PN}"
# The systemd-journal and render groups are created by the systemd recipe.
USERADD_DEPENDS = "systemd"
GROUPADD_PARAM:${PN} = "-r -g 900 castmill"
USERADD_PARAM:${PN} = " \
    -r -u 901 -g castmill -G video,render,input,audio -d /data/castmill/player -M -s /sbin/nologin player; \
    -r -u 902 -g castmill -G systemd-journal -d /data/castmill -M -s /sbin/nologin agent \
"

SYSTEMD_SERVICE:${PN} = " \
    castmill-data-setup.service castmill-data-layout.service \
    data.mount boot.mount var-lib-systemd.mount var-lib-NetworkManager.mount var-volatile-log-journal.mount \
    castmill-zram.service castmill-firewall.service castmill-banner.service \
"
SYSTEMD_AUTO_ENABLE = "enable"

do_install() {
    install -d ${D}${libexecdir}/castmill ${D}${systemd_system_unitdir}
    for f in castmill-data-setup castmill-zram castmill-banner; do
        sed -e 's|@UUID_DATA@|${CASTMILL_UUID_DATA}|' ${S}/$f > ${D}${libexecdir}/castmill/$f
        chmod 0755 ${D}${libexecdir}/castmill/$f
    done
    for u in castmill-data-setup.service castmill-data-layout.service data.mount boot.mount \
             var-lib-systemd.mount var-lib-NetworkManager.mount var-volatile-log-journal.mount \
             castmill-zram.service castmill-firewall.service castmill-banner.service; do
        sed -e 's|@UUID_DATA@|${CASTMILL_UUID_DATA}|' -e 's|@UUID_BOOT@|${CASTMILL_UUID_BOOT}|' \
            ${S}/$u > ${D}${systemd_system_unitdir}/$u
        chmod 0644 ${D}${systemd_system_unitdir}/$u
    done

    install -D -m 0644 ${S}/castmill-data.tmpfiles ${D}${nonarch_libdir}/tmpfiles.d/castmill-data.conf
    install -D -m 0644 ${S}/system.conf ${D}${systemd_unitdir}/system.conf.d/50-castmill.conf
    install -D -m 0644 ${S}/journald.conf ${D}${systemd_unitdir}/journald.conf.d/50-castmill.conf
    install -D -m 0644 ${S}/journal-flush.conf ${D}${systemd_system_unitdir}/systemd-journal-flush.service.d/50-castmill.conf
    install -D -m 0644 ${S}/rauc.conf ${D}${systemd_system_unitdir}/rauc.service.d/50-castmill.conf
    install -D -m 0644 ${S}/sysctl.conf ${D}${nonarch_libdir}/sysctl.d/50-castmill.conf

    install -D -m 0644 ${CASTMILL_LOGO} ${D}${datadir}/castmill/logo.txt
    install -D -m 0644 ${S}/castmill.nft ${D}${sysconfdir}/nftables/castmill.nft
    install -d ${D}${sysconfdir}/nftables/castmill-input.d
    install -D -m 0644 ${S}/NetworkManager.conf ${D}${sysconfdir}/NetworkManager/conf.d/50-castmill.conf
    install -D -m 0644 ${S}/castmill.rules ${D}${datadir}/polkit-1/rules.d/50-castmill.rules
    install -D -m 0644 ${S}/castmill-rauc.dbus.conf ${D}${datadir}/dbus-1/system.d/castmill-rauc.conf

    # Read-only /etc: point the few runtime-configurable files at /data.
    install -d ${D}${sysconfdir}/systemd/timesyncd.conf.d
    ln -sf /data/castmill/config/timesyncd.conf ${D}${sysconfdir}/systemd/timesyncd.conf.d/50-castmill.conf
    # Wi-Fi country for cfg80211, written by castmill-agent provision.
    install -d ${D}${sysconfdir}/modprobe.d
    ln -sf /data/castmill/config/wifi-country.conf ${D}${sysconfdir}/modprobe.d/castmill-wifi-country.conf
    install -d ${D}/data
}
do_install[vardeps] += "CASTMILL_UUID_DATA CASTMILL_UUID_BOOT CASTMILL_LOGO"
do_install[file-checksums] += "${CASTMILL_LOGO}:True"

# /etc/localtime -> /data (timezone configured from Castmill). Done at rootfs
# time because other packages (tzdata) also manage /etc/localtime.
pkg_postinst:${PN}() {
    ln -sf /data/castmill/config/agent/localtime $D${sysconfdir}/localtime
}

FILES:${PN} += " \
    ${libexecdir}/castmill ${systemd_system_unitdir} ${systemd_unitdir}/system.conf.d \
    ${systemd_unitdir}/journald.conf.d ${nonarch_libdir}/tmpfiles.d ${nonarch_libdir}/sysctl.d \
    ${datadir}/polkit-1 ${datadir}/dbus-1 ${datadir}/castmill /data \
"

RDEPENDS:${PN} = " \
    e2fsprogs-mke2fs e2fsprogs-e2fsck e2fsprogs-resize2fs \
    util-linux-sfdisk util-linux-partx util-linux-blkid util-linux-mount util-linux-mkswap util-linux-swapon \
    nftables networkmanager polkit kmod tzdata \
    iw wireless-regdb-static \
"
