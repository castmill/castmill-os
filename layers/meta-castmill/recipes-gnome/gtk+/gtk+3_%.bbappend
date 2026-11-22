# GTK3 is only here because the prebuilt Electron links against it. Drop the
# icon themes and image loaders it recommends: the Adwaita icon theme and
# the demo's SVG support pull in librsvg and with it the whole Rust toolchain.
GTKBASE_RRECOMMENDS = "liberation-fonts gdk-pixbuf-loader-png shared-mime-info"
RDEPENDS:${PN}-demo:remove = "librsvg-gtk"
