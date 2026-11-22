SUMMARY = "Castmill OS device agent"
DESCRIPTION = "Native side of the Castmill device app (castmillOS bridge), \
A/B updates through RAUC with maintenance windows, health checks, \
provisioning and player supervision. See docs/castmill-integration.md."
HOMEPAGE = "https://github.com/castmill/castmill-os"
# Agent code: Apache-2.0 (ADR-0007). Vendored Go modules: MIT, ISC,
# BSD-2-Clause, BSD-3-Clause.
# TODO: add per-module LIC_FILES_CHKSUM entries (generated licenses .inc).
LICENSE = "Apache-2.0 & MIT & ISC & BSD-2-Clause & BSD-3-Clause"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/Apache-2.0;md5=89aea4e17d99a7cacdbeed46a0096b10"

GO_IMPORT = "github.com/castmill/castmill-os/agent"

# Source: agent/ in this repository.
FILESEXTRAPATHS:prepend := "${CASTMILL_REPO_DIR}:${THISDIR}/files:"
SRC_URI = " \
    file://agent;subdir=${BP}/src/github.com/castmill/castmill-os \
    file://agent.toml.in \
    file://castmill-agent.service \
    file://castmill-health.service \
    file://castmill-health-report.service \
    file://castmill-health-report.timer \
    file://castmill-provision.service \
"
require castmill-agent-go-mods.inc

inherit go-mod systemd

# Modules come from the fetcher (gomod://), never from the network at build time.
export GOPROXY = "off"
# No -s -w: the build strips the binary and keeps symbols in the -dbg package.
GO_EXTRA_LDFLAGS = "-X ${GO_IMPORT}/internal/agent.Version=${PV}"

do_compile() {
    cd ${B}/src/${GO_IMPORT}
    export TMPDIR="${GOTMPDIR}"
    ${GO} build ${GOBUILDFLAGS} -o ${B}/bin/castmill-agent ./cmd/castmill-agent
}

CASTMILL_AGENT_DEV = "${@'true' if d.getVar('CASTMILL_DEV_IMAGE') == '1' else 'false'}"

do_install() {
    install -D -m 0755 ${B}/bin/castmill-agent ${D}${bindir}/castmill-agent

    install -d ${D}${sysconfdir}/castmill
    ca_file=""
    if [ -n "${CASTMILL_BACKEND_CA}" ]; then
        install -m 0644 ${CASTMILL_BACKEND_CA} ${D}${sysconfdir}/castmill/backend-ca.pem
        ca_file="${sysconfdir}/castmill/backend-ca.pem"
    fi
    sed -e 's|@BACKEND_URL@|${CASTMILL_BACKEND_URL}|' \
        -e "s|@BACKEND_CA@|$ca_file|" \
        -e 's|@DEV@|${CASTMILL_AGENT_DEV}|g' \
        -e 's|@MACHINE@|${MACHINE}|' \
        -e 's|@COMPATIBLE@|${CASTMILL_COMPATIBLE}|' \
        ${UNPACKDIR}/agent.toml.in > ${D}${sysconfdir}/castmill/agent.toml
    chmod 0644 ${D}${sysconfdir}/castmill/agent.toml

    install -d ${D}${systemd_system_unitdir}
    for u in castmill-agent.service castmill-health.service castmill-health-report.service \
             castmill-health-report.timer castmill-provision.service; do
        install -m 0644 ${UNPACKDIR}/$u ${D}${systemd_system_unitdir}/
    done
}
do_install[vardeps] += "CASTMILL_BACKEND_URL CASTMILL_BACKEND_CA CASTMILL_DEV_IMAGE CASTMILL_COMPATIBLE"

SYSTEMD_SERVICE:${PN} = "castmill-agent.service castmill-health.service castmill-health-report.timer castmill-provision.service"
# castmill-health-report.service is started by its timer, not enabled itself.
FILES:${PN} += "${systemd_system_unitdir}"

RDEPENDS:${PN} = "castmill-base"
