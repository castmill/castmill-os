'use strict';
// Pure decision logic for the kiosk shell (no Electron imports, unit-tested).

/**
 * What the window should show for an agent state: the Castmill device app
 * (player_url), or the local status page.
 * @returns {{view: 'status'|'content', url?: string, reason?: string}}
 */
function decideView(state) {
  if (!state || !state.player_url) return { view: 'status', reason: 'starting' };
  if (!isAllowedScheme(state.player_url, Boolean(state.dev_image))) {
    return { view: 'status', reason: 'invalid-player-url' };
  }
  return { view: 'content', url: state.player_url };
}

/** https everywhere; plain http only for loopback or on development images. */
function isAllowedScheme(u, allowHttp = false) {
  try {
    const p = new URL(u);
    return p.protocol === 'https:' || (p.protocol === 'http:' && (allowHttp || isLoopback(p.hostname)));
  } catch {
    return false;
  }
}

function isLoopback(host) {
  return host === 'localhost' || host === '127.0.0.1' || host === '[::1]';
}

function originOf(u) {
  try {
    return new URL(u).origin;
  } catch {
    return null;
  }
}

/**
 * Navigation is limited to the player's origin plus explicitly allowed content
 * origins (spec 04, "Least privilege").
 */
function isNavigationAllowed(target, playerURL, extraOrigins = [], allowHttp = false) {
  if (!isAllowedScheme(target, allowHttp)) return false;
  const allowed = new Set(extraOrigins);
  const po = originOf(playerURL);
  if (po) allowed.add(po);
  return allowed.has(originOf(target));
}

/**
 * The castmillOS bridge only answers the main frame of the Castmill device
 * app: the same origin as the player URL the agent configured.
 */
function isTrustedBridgeSender(frameURL, playerURL, allowHttp = false) {
  const po = originOf(playerURL);
  return Boolean(po) && isAllowedScheme(frameURL, allowHttp) && originOf(frameURL) === po;
}

/** Parses "--castmill-output=HDMI-A-1". */
function outputFromArgv(argv) {
  for (const a of argv) {
    const m = /^--castmill-output=([A-Za-z0-9_.-]+)$/.exec(a);
    if (m) return m[1];
  }
  return process.env.CASTMILL_OUTPUT || 'default';
}

module.exports = { decideView, isNavigationAllowed, isTrustedBridgeSender, outputFromArgv, isAllowedScheme };
