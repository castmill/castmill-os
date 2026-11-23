# Only the GPU drivers a Castmill OS target needs (minimal footprint).
#
# - No GLX: Weston and Electron use EGL/GLES; Electron only needs the X11
#   client libraries, not Mesa's GLX.
# - x86-64: Intel only: iris (Gen 8 and newer: Broadwell to Arc) and crocus
#   (Gen 4-7). AMD/nouveau/VMware drivers and the target LLVM (llvmpipe,
#   radeonsi, ~100 MB) are dropped. OE-core only builds iris together with
#   gallium-llvm, but iris doesn't use LLVM: its OpenCL kernels are compiled
#   at build time by mesa-tools-native (libclc). So iris is added here.
#   AMD (radeonsi) comes back with tier-2 hardware.
# - Vulkan is not a distro feature, so no Vulkan drivers or zink.
PACKAGECONFIG:remove = "x11 zink"
PACKAGECONFIG:remove:x86-64 = "gallium-llvm amd nouveau svga"
GALLIUMDRIVERS:append:x86-64 = ",iris"
