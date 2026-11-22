# 07 — Device management and remote access

> **Revised by [ADR-0008](../adr/0008-reuse-castmill-device-stack.md):** Castmill OS uses Castmill's own device app and device protocol (pincode pairing, device token, `/socket` channels, commands, telemetry). `castmill-agent` no longer enrolls or talks to the backend; it serves the native side of the device app through the `castmillOS` bridge and handles OS updates, health and provisioning. See [Castmill integration](../castmill-integration.md). Sections below that describe the agent's own enrollment, WebSocket and commands are superseded; screenshots, log upload and remote shell become Castmill platform features.

Status: Draft v0.1 · Related: MGT-*

## castmill-agent

A small daemon (language to be decided; Go or Rust preferred for a single static binary with little memory use), separate from the player.

### Responsibilities

- **Enrollment:** show the pairing code (through the player, or a fallback screen when the player is down), get the device certificate.
- **Connection:** persistent outbound WebSocket to the Castmill backend over mutual TLS, with exponential back-off.
- **Updates:** carry out [03](03-updates.md) through RAUC D-Bus; enforce maintenance windows; report status.
- **Configuration:** network, display layout, display power schedules, time zone, proxy; applied atomically and stored under `/data`.
- **Telemetry:** CPU, memory, temperature, throttling, storage use and wear, uptime, versions, player health, dropped video frames.
- **Commands:** reboot, restart player, screenshot (per output, via `grim`), collect logs (journal ring buffer, compressed upload), factory reset (needs confirmation in the UI).
- **Remote shell** (below).

### Fallback screen

If the player can't start (for example a broken update before rollback, or failed enrollment), the agent shows a minimal native status screen: device ID, network state, pairing code, error. This means a support conversation is always possible.

## Remote shell

- **Off by default.** Opened only when an operator with the right Castmill permission asks for it, for a single device.
- Carried over the agent's existing outbound connection (WebSocket-tunnelled PTY), so no inbound ports or VPN are needed.
- **Time-limited** (for example 30 minutes, extendable) and closes automatically.
- Every session is **audited**: who, when, and optionally a full session recording kept in Castmill.
- Runs as a restricted `support` user. Getting root needs a second confirmation step. (To be decided: whether root is allowed at all on production devices.)
- An organization can disable remote shell entirely.

## Remote view (later)

- Live view of a screen through periodic screenshots in v1. Real-time WebRTC streaming (GStreamer `webrtcbin`) MAY come later.

## Logs

- Structured logs from agent, player and system go to the journal.
- Uploaded on request or automatically after a rollback or crash loop.
- Sensitive data (tokens, Wi-Fi keys) is never logged.

## Backend counterpart (Castmill)

- Device view: OS version, slot status, health, last rollback reason, telemetry graphs.
- Fleet view: versions by channel, rollout progress, failure rates.
- Actions: reboot, screenshot, logs, shell, force update, pin version, factory reset.
