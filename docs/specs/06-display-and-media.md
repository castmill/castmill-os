# 06 — Display, graphics and media

Status: Draft v0.1 · Related: MED-*, [ADR-0003](../adr/0003-electron-player-runtime.md), [ADR-0004](../adr/0004-sway-kiosk-compositor.md)

## Graphics stack

- Kernel modesetting (KMS/DRM). **No X11.**
- Mesa: `iris` (Intel Gen8+), `v3d`/`vc4` (Raspberry Pi).
- Wayland compositor in a locked-down kiosk configuration: **Sway** or **Weston kiosk-shell** (ADR-0004, decided in Phase 0).
- Electron runs as a native Wayland client (`--ozone-platform=wayland`).

## Multiple outputs

- The player opens one `BrowserWindow` per output. The compositor pins each window fullscreen to its output (Sway `for_window` rules or Weston kiosk-shell per-output app IDs) (matched by app ID and title, for example `castmill-output-HDMI-A-1`).
- Per output: mode (resolution and refresh rate), rotation (0/90/180/270), scaling, and a forced mode when the EDID is unreliable.
- Modes: **independent** (different content per screen, v1) and **spanned** (one canvas across outputs, later; needed for video walls).
- Configuration comes from Castmill through the agent and is applied through the compositor's IPC without a reboot.

## Video decoding

### Strategy

1. **v1: Chromium's own hardware decoding inside Electron.** Supported per platform after testing (table below).
2. **Per-device content profiles.** At enrollment and after each update, the device reports its decode capabilities (MED-007). The Castmill transcoder serves the best rendition for each device.
3. **Later, if needed: native video path** (`castmill-videod`). The player hands full-zone videos to a GStreamer pipeline (VA-API on Intel, V4L2 on Pi) that draws into a Wayland surface below a transparent Electron window. Trade-off: CSS transforms, rounded corners and blending don't apply to those videos, and transitions need coordination.

### Platform matrix (to be confirmed in the prototype)

| Platform | Decode API | H.264 | HEVC | VP9 | AV1 | Notes |
|---|---|---|---|---|---|---|
| Intel N100/N150 (Gen12) | VA-API (iHD) | HW 4K | HW 4K | HW 4K | HW 4K | Chromium VA-API on Linux needs flags (`VaapiVideoDecodeLinuxGL`) and isn't officially supported. Must be verified on every Electron upgrade. |
| Raspberry Pi 5 | V4L2 stateless (HEVC only) | CPU (fine at 1080p) | HW 4K60 | CPU | CPU | The Pi 5 has no H.264 hardware decoder. A manual test showed smooth Electron playback on a Pi (model, codec and decoder still to be confirmed). |
| Raspberry Pi 4 | V4L2 (stateful H.264, stateless HEVC) | HW 1080p | HW 4K | CPU | – | Tier 2 |

### Verification method (for every platform and Electron version)

- `chrome://media-internals` → `video_decoder` name (hardware: `VaapiVideoDecoder` / `V4L2VideoDecoder`; software: `FFmpegVideoDecoder`, `Dav1dVideoDecoder`).
- `app.getGPUInfo('complete')` → supported hardware decode profiles.
- Decoder device in use by the GPU process (`lsof /dev/video*` / `/dev/dri/renderD*`).
- CPU use and `HTMLVideoElement.getVideoPlaybackQuality().droppedVideoFrames` during a 10-minute playback.

### Recommended content profiles (initial)

| Device profile | Video renditions |
|---|---|
| `intel-gen12` | H.264 or HEVC, up to 4K60 |
| `rpi5` | HEVC up to 4K60; H.264 up to 1080p60 |
| `rpi4` | H.264 up to 1080p; HEVC up to 4K30 |

## Electron configuration

- Flags are set by the OS per platform (in a generated `electron-flags.conf`), not hard-coded in the player.
- GPU blocklist overrides only where tested.
- Telemetry: GPU feature status and per-video dropped frames reported to the agent.

## Audio

- PipeWire with HDMI audio output per display. Selected output configurable from Castmill.

## Display power

- HDMI-CEC (`cec-ctl`) for on/off/standby and input switching, where the display supports it.
- DDC/CI (`ddcutil`) for monitors that don't support CEC.
- Schedules come from Castmill. The device keeps running while the screen is off.
