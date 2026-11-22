# weston-init isn't installed (castmill-session starts Weston), but it is a
# recommendation of weston and still gets built: don't pull in Xwayland just
# because "x11" is a distro feature (needed only for Electron's libraries).
PACKAGECONFIG:remove = "xwayland"
