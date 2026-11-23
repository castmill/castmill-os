# Chromium loads only libva.so.2 and libva-drm.so.2 (VA-API on DRM render
# nodes, also under Wayland). No X11/GLX/Wayland back-ends: the GLX one
# would need Mesa's GLX, which Castmill OS doesn't build.
PACKAGECONFIG:remove = "glx x11 wayland"
