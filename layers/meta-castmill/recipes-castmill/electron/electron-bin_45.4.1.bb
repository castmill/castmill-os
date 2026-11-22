SUMMARY = "Electron runtime (official prebuilt release)"
DESCRIPTION = "Prebuilt Electron for the Castmill player (ADR-0003). Built \
from the official release archives rather than from source to keep build \
times within the CI limits (spec 10, open question)."
HOMEPAGE = "https://www.electronjs.org/"
LICENSE = "MIT & BSD-3-Clause & LGPL-2.1-or-later"
LIC_FILES_CHKSUM = "file://LICENSE;md5=4d42118d35941e0f664dddbd83f633c5 \
                    file://LICENSES.chromium.html;md5=4e9c321282616adb27415f96205f0eee"

ELECTRON_ARCH = ""
ELECTRON_ARCH:x86-64 = "x64"
ELECTRON_ARCH:aarch64 = "arm64"

SRC_URI = "https://github.com/electron/electron/releases/download/v${PV}/electron-v${PV}-linux-${ELECTRON_ARCH}.zip;subdir=${BP};name=${ELECTRON_ARCH}"
SRC_URI[x64.sha256sum] = "e1d077c7a087d7e344350ea0c5cd2b7dc5af3d9bf7979474f38e0fc1a560b875"
SRC_URI[arm64.sha256sum] = "3da04c450fa682e3b80961e92a918a1c2a1a10796527a7e63dec7b8e466b6334"

COMPATIBLE_HOST = "(x86_64|aarch64).*-linux"

# Libraries the prebuilt binary links against (ELF DT_NEEDED of electron
# v45.4.1). GTK3, CUPS and the X11 client libraries are required even when
# running as a native Wayland client.
DEPENDS = " \
    glib-2.0 nspr nss at-spi2-core cups dbus cairo gtk+3 pango \
    libx11 libxcomposite libxdamage libxext libxfixes libxrandr libxcb \
    virtual/libgbm expat libxkbcommon udev alsa-lib \
"

inherit bin_package

S = "${UNPACKDIR}/${BP}"

do_install() {
    install -d ${D}${libdir}/electron
    cp -R --no-preserve=ownership ${S}/. ${D}${libdir}/electron/
    # The default app and the setuid sandbox helper are not used: the player
    # ships its own app, and Chromium sandboxes with user namespaces.
    rm -f ${D}${libdir}/electron/resources/default_app.asar ${D}${libdir}/electron/chrome-sandbox
    chmod 0755 ${D}${libdir}/electron/electron ${D}${libdir}/electron/chrome_crashpad_handler
}

FILES:${PN} = "${libdir}/electron"
# Bundled private libraries (FFmpeg, SwiftShader, Vulkan loader).
PRIVATE_LIBS = "libffmpeg.so libvk_swiftshader.so libvulkan.so.1"
INSANE_SKIP:${PN} = "already-stripped ldflags dev-so file-rdeps libdir"
INHIBIT_PACKAGE_STRIP = "1"
INHIBIT_PACKAGE_DEBUG_SPLIT = "1"
INHIBIT_SYSROOT_STRIP = "1"

RDEPENDS:${PN} += "liberation-fonts fontconfig"
