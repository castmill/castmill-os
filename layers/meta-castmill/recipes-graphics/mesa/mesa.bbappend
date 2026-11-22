# Only the GPU drivers a Castmill OS target needs (minimal footprint).
#
# - No GLX: Weston and Electron use EGL/GLES; Electron only needs the X11
#   client libraries, not Mesa's GLX.
# - x86-64: Intel (iris) only. AMD/nouveau/VMware drivers and the target LLVM
#   (llvmpipe, radeonsi) are dropped; Electron falls back to its bundled
#   SwiftShader. AMD (radeonsi) comes back with tier-2 hardware.
# - Vulkan is not a distro feature, so no Vulkan drivers or zink.
PACKAGECONFIG:remove = "x11 zink"
PACKAGECONFIG:remove:x86-64 = "gallium-llvm amd nouveau svga"
