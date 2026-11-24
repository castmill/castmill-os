# ADR-0004: Kiosk compositor for v1 (Sway or Weston kiosk-shell)

- Status: Accepted: **Weston kiosk-shell** (2026-11-24; first proposal: Sway)
- Date: 2026-11-22, decided 2026-11-24

> **Decision (2026-11-24):** Castmill OS v1 uses **Weston kiosk-shell**, as implemented since v0.1: it's in OE-core, needs no extra layer, and runs one kiosk shell per output reliably in the development VM. Sway stays an option for spanned outputs and video walls (Phase 3). The original proposal below is kept for the record.

## Context

The player needs a Wayland compositor that runs one fullscreen Electron window per output, supports per-output modes and rotation, survives hotplug, and can be controlled by the agent (outputs, screenshots).

## Original proposal (not adopted)

Use **Sway** with a locked-down configuration: no bar, no keybindings except a dev-image escape, `for_window` rules pinning player windows to outputs, IPC restricted to the `player` and `agent` users. Screenshots via `grim`.

## Alternatives considered

- **Cage:** minimal single-app kiosk compositor; ideal for one screen, but limited control over placing several windows on several outputs.
- **Weston (kiosk shell):** solid reference compositor; less convenient runtime IPC for output management.
- **Custom compositor (wlroots or Smithay):** maximum control (video walls, native video planes), but significant development and maintenance cost.

## Update (Yocto 6.0 "Wrynose" layer check)

Sway and wlroots are **not packaged in the Wrynose core layers** (OE-core, meta-openembedded). The usual third-party layer for them (meta-wayland) moved off GitHub, and Wrynose support hasn't been confirmed. **Weston** is in OE-core, and its **kiosk-shell** can assign applications to specific outputs.

Options for Phase 0:

1. **Weston kiosk-shell** (no extra layer): fullscreen apps pinned to outputs by app ID. Less runtime control over outputs than Sway IPC; screenshots and output changes need Weston-specific tooling.
2. **Sway** with our own wlroots/Sway recipes in `meta-castmill` (or a maintained third-party layer): best runtime control, at the cost of maintaining the recipes.

Decide after prototyping both with the Electron player on multiple outputs.

## Initial implementation (v0.1)

The first implementation uses **Weston kiosk-shell**, because it needs no extra layer on Wrynose. This is not the final decision; the Phase 0 comparison with Sway still stands. Findings so far:

- kiosk-shell assigns windows to outputs by Wayland **app ID**, and an Electron process has a single app ID. The player therefore runs **one Electron process per output** (`castmill-player@<output>.service`, app ID `castmill-output-<output>`). This isolates outputs from each other (one crash doesn't blank every screen) but costs memory per output and duplicates the content cache. Sway could match by window title and keep one process.
- Weston has no runtime output IPC: a mode or rotation change regenerates `weston.ini` and restarts the session (a few seconds of black screen).
- Screenshots are taken by the player (`webContents.capturePage()`) instead of the compositor, so they work the same with either compositor.
- To verify on hardware: Electron/Chromium derives the Wayland app ID from the desktop file name (`app.setDesktopName`).

## Consequences

- One Electron process per output (memory per output, separate caches), see above.
- Output mode and rotation changes restart the session (a few seconds of black screen).
- Revisit when spanned outputs / video walls or the native video path need tighter control: Sway (own wlroots/Sway recipes) or a custom wlroots compositor.
