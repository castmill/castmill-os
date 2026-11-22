# ADR-0003: Electron as the player runtime for v1

- Status: Accepted (short term)
- Date: 2026-11-22

## Context

The current Castmill Linux player is built on Electron. Rewriting it for another engine (for example WPE WebKit) is not realistic in the short term. Hardware video decoding in Chromium on Linux depends on the platform:

- **Intel:** VA-API works with flags but isn't officially supported by Chromium on Linux.
- **Raspberry Pi:** a manual test showed smooth Electron playback with apparent hardware decoding. The exact Pi model, codec, OS and decoder still need to be confirmed. The Pi 5 has hardware decoding for HEVC only.

## Decision

Ship the Electron player inside the OS image for v1, running as a native Wayland client under the kiosk compositor. Platform-specific Chromium flags are set by the OS. Electron's auto-updater is disabled; the player is updated with the OS.

## Consequences

- Fastest path to a working product, reusing existing player code.
- Larger image and memory use than a WPE-based runtime.
- **Security:** Electron/Chromium upgrades must be shipped promptly; this is a standing release task.
- **Media:** hardware decoding must be verified per platform and per Electron upgrade in CI (see spec 06). Per-device content profiles in the Castmill transcoder are required.
- **Escape hatches:** native GStreamer video path (`castmill-videod`) and, in the long term, evaluating WPE WebKit.
