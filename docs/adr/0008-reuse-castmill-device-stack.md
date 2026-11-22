# ADR-0008: Reuse Castmill's device stack

- Status: Accepted
- Date: 2026-11-22
- Revises: spec 07 (device management), parts of specs 03, 04 and 08; supersedes the draft device API (`/api/os/v1`)

## Context

The first implementation of Castmill OS (v0.1) came with its own device protocol: enrollment with a pairing code and a CSR, per-device X.509 certificates, mutual TLS, and a WebSocket for commands, configuration and telemetry (`/api/os/v1`). It was designed without looking at the Castmill platform's existing device stack, which already provides:

- **Pairing:** the backend generates a pincode; the device shows it with a QR code; an operator claims it in the dashboard (`devices_registrations`, `Devices.register_device/4`).
- **Authentication:** device ID plus a bearer token (stored as an Argon2 hash).
- **Live connection:** Phoenix socket `/socket`, channel `devices:<id>`, heartbeats and online/offline events.
- **Commands and data:** `refresh`, `restart_app`, `restart_device`, `clear_cache`, `update_app`, `update_firmware`; telemetry, cache and timers over the socket; error reports and events in the dashboard.
- **Platforms:** the device app (`@castmill/device`) runs on every platform (web, Electron, Android, webOS) through a native `Machine` interface that each platform implements.

Running both stacks would mean two device registries, two pairings per screen, two authentication systems and duplicated dashboard features.

## Decision

Castmill OS is a **Castmill platform**, like the Electron, Android and webOS players:

- The kiosk shell loads Castmill's device app from the backend (`<backend>/`). The device app does pairing, content, commands and telemetry over Castmill's existing protocol.
- The kiosk shell exposes a small native bridge, `window.castmillOS`, that implements Castmill's `Machine` interface. It answers only the main frame of the Castmill origin. Castmill's `CastmillOSMachine` (in `@castmill/device`) uses it when present.
- `castmill-agent` keeps the OS duties: the native side of the bridge (machine ID, device info, telemetry, time zone, reboot), A/B updates, `castmill-health`, provisioning and player supervision. It drops its own enrollment, certificates, mTLS client and WebSocket.
- Each output registers as its own Castmill device (independent content per screen, MED-005).
- OS updates (release registry, desired version per device, update status) are added to the Castmill backend on top of the existing device authentication, in a follow-up change. Until then, updates are triggered locally (`castmill-agent update`).

The protocol between the kiosk shell, the agent and Castmill is described in [Castmill integration](../castmill-integration.md).

## Alternatives considered

- **Keep the separate OS protocol** (`/api/os/v1` with mTLS): stronger device identity, but two registries, double pairing, and duplicated commands and telemetry in the backend and dashboard.
- **Load the hosted web player with no native integration:** quickest, but no reboot, OS telemetry, stable machine ID or OS updates.

## Consequences

- Devices pair with Castmill exactly like other players; hardware images can be paired with the production backend as soon as it serves the device app.
- **SEC-005 changes:** device identity is Castmill's bearer token, not a per-device certificate with mTLS. Hardening the token (for example binding it to a TPM-held key) becomes a Castmill platform topic for all players.
- **Zero-touch bulk enrollment** (PRV-003, `castmill.conf` `[enrollment]`) needs Castmill support for organization tokens; until then devices pair with the pincode.
- **Screenshots and log upload** (MGT-001) don't exist in Castmill yet; they become Castmill features usable by all platforms.
- **The device app is served by the backend,** not shipped in the image. Castmill's service worker caches it for offline starts. Bundling it in the image remains an option if offline cold starts prove unreliable.
- The agent got smaller (one Go dependency fewer, no TLS client certificates).
